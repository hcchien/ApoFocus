//go:build !darwin && !linux && !freebsd && !openbsd && !netbsd && !dragonfly

package fileidentity

import "os"

func platformIdentity(_ os.FileInfo) (uint64, uint64, bool) { return 0, 0, false }
