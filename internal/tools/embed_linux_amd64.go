//go:build linux && amd64

package tools

import "embed"

//go:embed data/linux_amd64
var embedded embed.FS
