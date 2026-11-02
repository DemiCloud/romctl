//go:build darwin && arm64

package tools

import "embed"

//go:embed data/darwin_arm64
var embedded embed.FS
