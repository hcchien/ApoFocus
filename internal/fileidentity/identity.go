package fileidentity

import (
	"fmt"
	"os"
)

type Identity struct {
	FileID   string
	VolumeID string
}

func FromPath(path string) (Identity, error) {
	info, err := os.Stat(path)
	if err != nil {
		return Identity{}, err
	}
	device, inode, ok := platformIdentity(info)
	if !ok {
		return Identity{}, nil
	}
	return Identity{
		FileID:   fmt.Sprintf("%d:%d", device, inode),
		VolumeID: fmt.Sprintf("device:%d", device),
	}, nil
}
