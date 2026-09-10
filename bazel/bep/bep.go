// Package bep reduces a Bazel Build Event Protocol stream to one hash per
// target, over the digests of the outputs that target produced.
//
// Bazel already knows these digests: the digest function is sha256 (see
// .bazelrc, which pins it so a layer's key in the cache is its digest in a
// registry), so every output's content hash is in the action cache and the
// BEP reports it. Nothing here re-hashes a file; it reads out what the build
// already established.
//
// The point is to tell "Bazel will re-run this action" apart from "the bytes
// changed". target-determinator answers the first before a build, which is
// what a test gate needs. This answers the second afterwards: a change to a
// build tool re-runs every action downstream of it and can still produce
// identical output, and only a comparison of digests shows that.
package bep

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// outputGroup is the only group read: `bazel build` produces the default
// group, and the others appear only when a run asks for them, so including
// them would make the map depend on the flags of the run that wrote it.
const defaultOutputGroup = "default"

type setRef struct {
	ID string `json:"id"`
}

type namedSetOfFiles struct {
	Files []struct {
		Name   string `json:"name"`
		Digest string `json:"digest"`
	} `json:"files"`
	FileSets []setRef `json:"fileSets"`
}

type event struct {
	ID struct {
		NamedSet        *setRef `json:"namedSet"`
		TargetCompleted *struct {
			Label string `json:"label"`
		} `json:"targetCompleted"`
	} `json:"id"`
	NamedSetOfFiles *namedSetOfFiles `json:"namedSetOfFiles"`
	Completed       *struct {
		Success     bool `json:"success"`
		OutputGroup []struct {
			Name     string   `json:"name"`
			FileSets []setRef `json:"fileSets"`
		} `json:"outputGroup"`
	} `json:"completed"`
}

// OutputDigests maps each label that produced outputs to a hex sha256 over
// them. A label absent from the result produced nothing worth comparing:
// it failed, or its default output group is empty.
func OutputDigests(r io.Reader) (map[string]string, error) {
	sets := map[string]*namedSetOfFiles{}
	// A label can complete once per configuration, so the roots accumulate
	// rather than replace: the hash then covers the target as built
	// everywhere in this invocation.
	roots := map[string][]setRef{}

	scanner := bufio.NewScanner(r)
	// Some events (the command line, a large file set) run long past the
	// 64 KiB the scanner allows by default.
	scanner.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			return nil, fmt.Errorf("parsing build event: %w", err)
		}
		switch {
		case e.ID.NamedSet != nil && e.NamedSetOfFiles != nil:
			sets[e.ID.NamedSet.ID] = e.NamedSetOfFiles
		case e.ID.TargetCompleted != nil && e.Completed != nil && e.Completed.Success:
			for _, group := range e.Completed.OutputGroup {
				if group.Name == defaultOutputGroup {
					roots[e.ID.TargetCompleted.Label] = append(roots[e.ID.TargetCompleted.Label], group.FileSets...)
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading build events: %w", err)
	}

	digests := make(map[string]string, len(roots))
	for label, refs := range roots {
		if hash := hashOutputs(sets, refs); hash != "" {
			digests[label] = hash
		}
	}
	return digests, nil
}

// hashOutputs walks the file sets a target's outputs live in and hashes what
// it finds, or returns "" when it finds nothing.
func hashOutputs(sets map[string]*namedSetOfFiles, refs []setRef) string {
	// Keyed on name *and* digest: the same output name can arrive from two
	// configurations with different bytes, and both belong in the hash.
	// Deduplicated because file sets are a graph, not a tree, and one file
	// reached twice is still one file.
	seen := map[string]struct{}{}
	visited := map[string]struct{}{}

	var walk func(ref setRef)
	walk = func(ref setRef) {
		if _, done := visited[ref.ID]; done {
			return
		}
		visited[ref.ID] = struct{}{}
		set, ok := sets[ref.ID]
		if !ok {
			return
		}
		for _, f := range set.Files {
			seen[f.Name+"\x00"+f.Digest] = struct{}{}
		}
		for _, child := range set.FileSets {
			walk(child)
		}
	}
	for _, ref := range refs {
		walk(ref)
	}
	if len(seen) == 0 {
		return ""
	}

	entries := make([]string, 0, len(seen))
	for entry := range seen {
		entries = append(entries, entry)
	}
	sort.Strings(entries)

	sum := sha256.New()
	for _, entry := range entries {
		fmt.Fprintf(sum, "%s\n", entry)
	}
	return hex.EncodeToString(sum.Sum(nil))
}
