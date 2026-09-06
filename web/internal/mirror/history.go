package mirror

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	"github.com/arkeros/distroless/web/internal/attestation"
	"github.com/arkeros/distroless/web/internal/directory"
)

// ledgerRepository is where the families' tag ledgers live, beside the
// families themselves: <registry>/<prefix>/history, one tag per family. The
// `release` job writes them; see ADR 0017.
const ledgerRepository = "history"

// ledgerMediaType is the layer a ledger artifact carries: JSON Lines, one
// Move per line, appended and never rewritten.
const ledgerMediaType = "application/vnd.distroless.tag-ledger.v1+jsonl"

// Moves reads a family's tag ledger: every tag move `release` has recorded
// for it, in the order they were recorded. Empty, and not an error, for a
// family that has no ledger yet.
//
// Unverified, as Versions is: which build a tag named is registry metadata.
// The ledger's digest changes on every event, so a parsed ledger is cached by
// that digest and can never go stale, only be evicted.
func (c *Client) Moves(ctx context.Context, family string) ([]directory.Move, error) {
	repository, err := c.repository(ledgerRepository)
	if err != nil {
		return nil, fmt.Errorf("parsing ledger repository: %w", err)
	}
	options := append([]remote.Option{remote.WithContext(ctx)}, c.remoteOptions...)

	descriptor, err := c.puller.Head(ctx, repository.Tag(family))
	if err != nil {
		// No ledger is a family that has not been released since ledgers
		// began, not a failure to read one. Anything but a plain 404 is.
		var registry *transport.Error
		if errors.As(err, &registry) && registry.StatusCode == http.StatusNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("resolving ledger %s:%s: %w", repository, family, err)
	}
	digest := descriptor.Digest.String()
	if moves, ok := c.ledgers.Get(digest); ok {
		return moves, nil
	}

	image, err := remote.Image(repository.Digest(digest), options...)
	if err != nil {
		return nil, fmt.Errorf("reading ledger %s:%s: %w", repository, family, err)
	}
	layers, err := image.Layers()
	if err != nil {
		return nil, fmt.Errorf("reading ledger %s:%s: %w", repository, family, err)
	}
	for _, layer := range layers {
		mediaType, err := layer.MediaType()
		if err != nil {
			return nil, err
		}
		if string(mediaType) != ledgerMediaType {
			continue
		}
		reader, err := layer.Uncompressed()
		if err != nil {
			return nil, err
		}
		blob, err := io.ReadAll(io.LimitReader(reader, maxAttestationBytes))
		reader.Close()
		if err != nil {
			return nil, err
		}
		moves, err := decodeLedger(blob)
		if err != nil {
			return nil, fmt.Errorf("ledger %s:%s: %w", repository, family, err)
		}
		c.ledgers.Add(digest, moves)
		return moves, nil
	}
	return nil, fmt.Errorf("ledger %s:%s carries no %s layer", repository, family, ledgerMediaType)
}

// ledgerEvent is one line of a ledger as `release` writes it.
type ledgerEvent struct {
	At     string `json:"at"`
	Tag    string `json:"tag"`
	Digest string `json:"digest"`
	Run    string `json:"run"`
}

// decodeLedger parses JSON Lines into Moves, in file order.
//
// One bad line fails the whole ledger rather than being skipped: a history
// with a hole in it draws as a tag that never moved, which is the one thing
// a history must not say by accident. The same rule the writer applies
// (//oci/publish:ledger.jq).
func decodeLedger(blob []byte) ([]directory.Move, error) {
	var moves []directory.Move
	scanner := bufio.NewScanner(bytes.NewReader(blob))
	scanner.Buffer(nil, maxAttestationBytes)
	for line := 1; scanner.Scan(); line++ {
		text := strings.TrimSpace(scanner.Text())
		if text == "" {
			continue
		}
		var event ledgerEvent
		if err := json.Unmarshal([]byte(text), &event); err != nil {
			return nil, fmt.Errorf("line %d: %w", line, err)
		}
		at, err := time.Parse(time.RFC3339, event.At)
		if err != nil {
			return nil, fmt.Errorf("line %d: at: %w", line, err)
		}
		if event.Tag == "" || !strings.HasPrefix(event.Digest, "sha256:") {
			return nil, fmt.Errorf("line %d: not a tag move: need tag and a sha256: digest", line)
		}
		moves = append(moves, directory.Move{At: at, Tag: event.Tag, Digest: event.Digest, Run: event.Run})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return moves, nil
}

// Scans resolves family and ref and returns every verified scan record on the
// Digest behind them, oldest first, each with the newest VEX document
// applied: the digest's history, where Scan is its present.
//
// Today's VEX document rather than the one current at each scan. A statement
// is this project's verdict on a finding, and the verdict a reader wants on a
// week-old scan is the one that stands now — not the one a since-withdrawn
// document gave.
func (c *Client) Scans(ctx context.Context, family, ref string) (string, []*directory.Scan, error) {
	subject, digest, options, err := c.resolve(ctx, family, ref)
	if err != nil {
		return "", nil, err
	}
	if series, ok := c.series.Get(digest); ok {
		return digest, series, nil
	}

	found, err := c.predicates(subject, digest, options)
	if err != nil {
		return "", nil, err
	}
	series := make([]*directory.Scan, 0, len(found[attestation.Vuln]))
	for _, statement := range found[attestation.Vuln] {
		scan, err := decodeScan(statement.Predicate)
		if err != nil {
			slog.Warn("skipping undecodable scan record", "subject", subject, "error", err)
			continue
		}
		series = append(series, scan)
	}
	if len(series) == 0 {
		return "", nil, fmt.Errorf("no verified vulnerability scan attached to %s", subject)
	}
	if vex := newestStatement(found[attestation.OpenVEX]); vex != nil {
		for _, scan := range series {
			if err := suppress(scan, vex.Predicate); err != nil {
				slog.Warn("skipping VEX document", "subject", subject, "error", err)
				break
			}
		}
	}
	slices.SortStableFunc(series, func(a, b *directory.Scan) int { return a.Finished.Compare(b.Finished) })
	c.series.Add(digest, series)
	return digest, series, nil
}
