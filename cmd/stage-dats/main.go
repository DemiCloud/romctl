// Command stage-dats drives `make dats`: it matches whatever the
// maintainer has dropped under thirdparty/dats/<provider>/ (an entire
// unzipped upstream pack, typically) against systemdb.Dat, and copies
// just the needed files into internal/datdb/data/<provider>/ for
// embedding. All the actual matching/validation logic lives in
// internal/datdb.Stage; this is a thin wrapper, same convention as
// cmd/romctl's own command files. Not part of any release - a build-time
// maintainer tool only, run via `go run ./cmd/stage-dats`.
package main

import (
	"fmt"
	"os"

	"github.com/demicloud/romctl/internal/datdb"
)

func main() {
	results, err := datdb.Stage("thirdparty/dats", "internal/datdb/data")
	if err != nil {
		fmt.Fprintln(os.Stderr, "stage-dats:", err)
		os.Exit(1)
	}

	failed, missing := 0, 0
	for _, r := range results {
		switch r.Status {
		case "staged":
			fmt.Printf("staged     %-8s %s\n", r.Provider, r.Name)
		case "missing":
			missing++
		default:
			fmt.Fprintf(os.Stderr, "%-10s %-8s %-50s %s\n", r.Status, r.Provider, r.Name, r.Detail)
			failed++
		}
	}
	if missing > 0 {
		fmt.Printf("%d system(s) have no DAT staged yet (not in thirdparty/dats/) - fine if you don't need them\n", missing)
	}
	if failed > 0 {
		fmt.Fprintf(os.Stderr, "stage-dats: %d problem(s) above\n", failed)
		os.Exit(1)
	}
}
