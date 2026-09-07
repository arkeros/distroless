package tarballs

import (
	"context"
	"fmt"
	"reflect"
)

// Source resolves the newest release of a major line.
type Source interface {
	Latest(ctx context.Context, major string) (Line, error)
}

// NewSource maps a lockfile's `source` field to its resolver.
func NewSource(name string) (Source, error) {
	switch name {
	case "nodejs":
		return &NodeJS{BaseURL: "https://nodejs.org/dist"}, nil
	case "temurin":
		return &Temurin{BaseURL: "https://api.adoptium.net/v3"}, nil
	}
	return nil, fmt.Errorf("unknown tarball source %q", name)
}

// Change records one line moved by Update.
type Change struct {
	Major    string
	From, To string
}

func (c Change) String() string {
	return fmt.Sprintf("%s: %s -> %s", c.Major, c.From, c.To)
}

// Update moves every line of lock to its newest release and reports which
// ones changed. Lines are replaced wholesale, so a checksum change without
// a version change (an upstream re-publish) is picked up too.
func Update(ctx context.Context, lock *Lock, src Source) ([]Change, error) {
	var changes []Change
	for i, current := range lock.Lines {
		latest, err := src.Latest(ctx, current.Major)
		if err != nil {
			return nil, err
		}
		if reflect.DeepEqual(current, latest) {
			continue
		}
		changes = append(changes, Change{Major: current.Major, From: current.describe(), To: latest.describe()})
		lock.Lines[i] = latest
	}
	return changes, nil
}

func (l Line) describe() string {
	if l.Build == "" {
		return l.Version
	}
	return l.Version + "+" + l.Build
}
