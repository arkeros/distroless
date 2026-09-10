package bep_test

import (
	"strings"
	"testing"

	"github.com/arkeros/distroless/bazel/bep"
)

// Each line is one Build Event Protocol event, the shape
// `--build_event_json_file` writes: a `namedSet` carries files and may point
// at further sets, and a `targetCompleted` names the sets its output groups
// resolve to.
const events = `
{"id":{"namedSet":{"id":"leaf"}},"namedSetOfFiles":{"files":[{"name":"b.bin","digest":"bbb"}]}}
{"id":{"namedSet":{"id":"root"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"}],"fileSets":[{"id":"leaf"}]}}
{"id":{"namedSet":{"id":"other"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"}]}}
{"id":{"targetCompleted":{"label":"//pkg:nested"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"root"}]}]}}
{"id":{"targetCompleted":{"label":"//pkg:flat"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"other"}]}]}}
{"id":{"targetCompleted":{"label":"//pkg:failed"}},"completed":{"success":false,"outputGroup":[{"name":"default","fileSets":[{"id":"root"}]}]}}
{"id":{"targetCompleted":{"label":"//pkg:nooutputs"}},"completed":{"success":true}}
`

func digests(t *testing.T, lines string) map[string]string {
	t.Helper()
	got, err := bep.OutputDigests(strings.NewReader(lines))
	if err != nil {
		t.Fatalf("OutputDigests: %v", err)
	}
	return got
}

func TestResolvesNestedFileSets(t *testing.T) {
	got := digests(t, events)
	// //pkg:nested reaches a.bin directly and b.bin through the child set, so
	// it must not hash the same as //pkg:flat, which only has a.bin.
	if got["//pkg:nested"] == "" {
		t.Fatal("no hash for //pkg:nested")
	}
	if got["//pkg:nested"] == got["//pkg:flat"] {
		t.Error("a target with an extra output through a child file set hashes the same as one without it")
	}
}

func TestSkipsTargetsWithNothingToHash(t *testing.T) {
	got := digests(t, events)
	for _, label := range []string{"//pkg:failed", "//pkg:nooutputs"} {
		if _, ok := got[label]; ok {
			t.Errorf("%s should not appear: it has no successful outputs", label)
		}
	}
}

func TestIsOrderIndependent(t *testing.T) {
	// Bazel emits a target's file sets in whatever order the build produced
	// them; the same outputs must hash the same either way.
	shuffled := `
{"id":{"namedSet":{"id":"s"}},"namedSetOfFiles":{"files":[{"name":"b.bin","digest":"bbb"},{"name":"a.bin","digest":"aaa"}]}}
{"id":{"targetCompleted":{"label":"//pkg:t"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"s"}]}]}}
`
	ordered := `
{"id":{"namedSet":{"id":"s"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"},{"name":"b.bin","digest":"bbb"}]}}
{"id":{"targetCompleted":{"label":"//pkg:t"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"s"}]}]}}
`
	if digests(t, shuffled)["//pkg:t"] != digests(t, ordered)["//pkg:t"] {
		t.Error("output order changed the hash")
	}
}

func TestDigestChangeMovesTheHash(t *testing.T) {
	moved := strings.Replace(events, `"digest":"bbb"`, `"digest":"ccc"`, 1)
	if digests(t, events)["//pkg:nested"] == digests(t, moved)["//pkg:nested"] {
		t.Error("a changed output digest left the target hash alone")
	}
}

func TestOnlyTheDefaultOutputGroup(t *testing.T) {
	// `bazel build` produces the default group; the others appear only when
	// asked for, so including them would make the map depend on the flags of
	// the run that produced it.
	extra := `
{"id":{"namedSet":{"id":"d"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"}]}}
{"id":{"namedSet":{"id":"x"}},"namedSetOfFiles":{"files":[{"name":"x.json","digest":"xxx"}]}}
{"id":{"targetCompleted":{"label":"//pkg:t"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"d"}]},{"name":"deploy_manifest","fileSets":[{"id":"x"}]}]}}
`
	only := `
{"id":{"namedSet":{"id":"d"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"}]}}
{"id":{"targetCompleted":{"label":"//pkg:t"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"d"}]}]}}
`
	if digests(t, extra)["//pkg:t"] != digests(t, only)["//pkg:t"] {
		t.Error("a non-default output group changed the hash")
	}
}

func TestSurvivesADanglingFileSet(t *testing.T) {
	// A build killed mid-stream leaves a `targetCompleted` pointing at a set
	// whose event never arrived. Report what is there rather than failing the
	// whole map over one truncated target.
	truncated := `
{"id":{"namedSet":{"id":"present"}},"namedSetOfFiles":{"files":[{"name":"a.bin","digest":"aaa"}],"fileSets":[{"id":"never-emitted"}]}}
{"id":{"targetCompleted":{"label":"//pkg:t"}},"completed":{"success":true,"outputGroup":[{"name":"default","fileSets":[{"id":"present"},{"id":"also-missing"}]}]}}
`
	got := digests(t, truncated)
	if got["//pkg:t"] == "" {
		t.Error("a target with one resolvable file set and one dangling reference produced no hash")
	}
}
