//go:build !darwin && !linux

package backup

import "errors"

func diskSpace(string) (uint64, uint64, error) {
	return 0, 0, errors.New("disk space inspection is unavailable on this platform")
}
