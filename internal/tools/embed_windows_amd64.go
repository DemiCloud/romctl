//go:build windows && amd64

package tools

import "embed"

//go:embed data/windows_amd64
var embedded embed.FS
