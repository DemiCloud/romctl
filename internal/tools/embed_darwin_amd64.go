//go:build darwin && amd64

package tools

import "embed"

//go:embed data/darwin_amd64
var embedded embed.FS
