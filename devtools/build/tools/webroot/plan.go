package webroot

import (
	"slices"
)

// Changes is the work of one publish: what to write, then what to remove.
//
// The order is not a preference. Objects that name other objects have to be
// written after the ones they name and deleted before them, because a client
// reads them in that order too: it parses index.html, then fetches the chunks
// index.html named. Apply UploadOrder's waves in turn, then Delete.
type Changes struct {
	Upload []string
	Delete []string
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
	return Changes{Upload: upload, Delete: stale}
}
