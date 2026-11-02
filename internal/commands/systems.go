package commands

import (
	"fmt"
	"sort"

	"github.com/demicloud/romctl/internal/systemdb"
)

// Systems prints every known system and its class, sorted by name.
func Systems() {
	names := systemdb.Names()
	sort.Strings(names)
	for _, s := range names {
		fmt.Printf("%-16s %s\n", s, systemdb.ClassOf(s))
	}
}
