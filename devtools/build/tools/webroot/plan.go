package webroot

import (
	"slices"
	"time"
)

// Orphan is a bucket object the build no longer produces, together with when
// a publish first found it that way. A zero OrphanedAt means no publish has
// recorded it yet.
type Orphan struct {
	Name       string
	OrphanedAt time.Time
}

// Retire selects the orphans this publish has to date.
//
// A publish does not delete what it orphans. A client still on the page
// loaded the previous entry document, so it holds URLs for chunks this build
// no longer produces — and a lazy route's chunk is fetched when the user
// navigates to it, not when the page loaded. Wave ordering cannot help:
// ordering protects a client reading the *new* entry document, and this one
// is reading the old one. Only elapsed time does.
//
// Dating an object hands it to the bucket's lifecycle rule, which deletes it
// a retention window later — see `staticsite`. That keeps the retention
// where ADR 0009 put the rest of the bucket's lifecycle, in Terraform, and
// means orphans are collected whether or not anyone deploys again.
//
// An orphan already carrying a date is left alone. Every later publish sees
// it again until the bucket collects it, and re-dating would push its
// deletion out by a full window each time — an object orphaned once would
// then outlive any retention it was given.
func Retire(orphans []Orphan) []string {
	var stamp []string
	for _, o := range orphans {
		if o.OrphanedAt.IsZero() {
			stamp = append(stamp, o.Name)
		}
	}
	return stamp
}

// Changes is the work of one publish: what to write, then what the build has
// stopped producing.
//
// The write order is not a preference. Objects that name other objects have
// to be written after the ones they name, because a client reads them in
// that order too: it parses index.html, then fetches the chunks index.html
// named. Apply UploadOrder's waves in turn.
//
// Orphaned is not a delete list. A publish dates those objects and leaves
// them standing for the bucket's lifecycle rule to collect, because a client
// that loaded the previous entry document still fetches them — see Retire.
type Changes struct {
	Upload   []string
	Orphaned []string
}

// UploadOrder splits Upload into the two waves a publish writes in turn.
//
// Content-addressed objects go first and have to land in full before the
// second wave begins: the mutable objects — index.html, the unhashed entry
// bundle, the stylesheets — are how a client discovers every hashed URL, so
// a client that fetched a new one while its chunks were still uploading
// would resolve a script tag to nothing.
//
// One barrier is enough because of what a client can reach, not because of
// what the first wave names — the chunks do name each other, since a lazy
// route chunk imports the shared vendor one. A name written in the first
// wave is either a hash nothing published yet links to, or a hash that was
// already there and whose bytes are identical. Either way, no URL a live
// client holds changes meaning until the second wave republishes the
// objects that hand those URLs out.
//
// Within a wave the writes are independent and order does not matter.
func (c Changes) UploadOrder(rules Rules) (immutable, mutable []string) {
	for _, name := range c.Upload {
		if rules.Immutable(name) {
			immutable = append(immutable, name)
		} else {
			mutable = append(mutable, name)
		}
	}
	return immutable, mutable
}

// Plan diffs the built webroot against what the bucket currently holds.
// Both arguments are bucket-relative object names; the result is sorted, so
// a publish's log reads the same way twice.
//
// Every local file is uploaded, including ones already present. The
// content-addressed majority are byte-identical under a given name, so
// rewriting them changes nothing observable, and the rest — index.html and
// the unhashed entry bundle — are precisely the objects whose bytes change
// under a stable name. A hash comparison would save bandwidth on a payload
// measured in kilobytes and introduce a way for a deploy to silently not
// happen.
func Plan(local, remote []string) Changes {
	built := make(map[string]bool, len(local))
	for _, name := range local {
		built[name] = true
	}

	var stale []string
	for _, name := range remote {
		if !built[name] {
			stale = append(stale, name)
		}
	}

	upload := slices.Clone(local)
	slices.Sort(upload)
	slices.Sort(stale)
	return Changes{Upload: upload, Orphaned: stale}
}
