package datdb

import (
	"io/fs"
	"sort"

	"github.com/demicloud/romctl/internal/systemdb"
)

// Status is one system's DAT coverage: whether a snapshot is embedded for
// it, and (if so) that snapshot's own version/date, read straight out of
// its header - there's no separately tracked version, see this package's
// doc comment.
type Status struct {
	Provider string
	System   string // romctl's own system directory name
	Name     string // the provider's own pack name
	Version  string
	Date     string
	Embedded bool
}

// AllStatuses reports Status for every system with a DatSource entry,
// sorted by provider then system - read-only, like tools.AllStatuses:
// no override directory is consulted, only the embedded set, since this
// is about what this specific romctl build shipped with.
func AllStatuses() []Status {
	out := make([]Status, 0, len(systemdb.Dat))
	for sys, src := range systemdb.Dat {
		st := Status{Provider: src.Provider, System: sys, Name: src.System}
		if data, err := fs.ReadFile(embedded, pathDataPath(src)); err == nil {
			if df, err := Parse(data); err == nil {
				st.Embedded = true
				st.Version = df.Header.Version
				st.Date = df.Header.Date
			}
		}
		out = append(out, st)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Provider != out[j].Provider {
			return out[i].Provider < out[j].Provider
		}
		return out[i].System < out[j].System
	})
	return out
}
