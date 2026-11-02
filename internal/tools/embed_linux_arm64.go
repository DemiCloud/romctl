//go:build linux && arm64

package tools

import "embed"

//go:embed data/linux_arm64
var embedded embed.FS
