package commands

import (
	"crypto/md5"
	"encoding/hex"
)

func md5Hex(data []byte) string {
	sum := md5.Sum(data)
	return hex.EncodeToString(sum[:])
}
