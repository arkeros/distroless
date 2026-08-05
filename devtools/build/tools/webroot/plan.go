package webroot

import (
	"slices"
)

// Changes is the work of one publish: what to write, then what to remove.
//
// The order is not a preference. A client that has already parsed
// index.html holds URLs for the chunks it names, so the new objects have to
// exist before the old ones stop existing — otherwise a request in flight
// during the deploy resolves to nothing. Apply Upload in full before Delete.
type Changes struct {
	Upload []string
	Delete []string
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
