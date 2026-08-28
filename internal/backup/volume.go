package backup

import (
	"context"
	"encoding/xml"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

func actualVolumeUUID(ctx context.Context, path string) (string, error) {
	if runtime.GOOS != "darwin" {
		return "", errors.New("volume UUID inspection is available only on macOS")
	}
	clean := filepath.Clean(path)
	parts := strings.Split(strings.TrimPrefix(clean, string(filepath.Separator)), string(filepath.Separator))
	if len(parts) >= 2 && parts[0] == "Volumes" {
		clean = filepath.Join(string(filepath.Separator), "Volumes", parts[1])
	}
	output, err := exec.CommandContext(ctx, "/usr/sbin/diskutil", "info", "-plist", clean).Output()
	if err != nil {
		return "", err
	}
	decoder := xml.NewDecoder(strings.NewReader(string(output)))
	lastKey := ""
	for {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return "", tokenErr
		}
		start, ok := token.(xml.StartElement)
		if !ok {
			continue
		}
		switch start.Name.Local {
		case "key":
			if err := decoder.DecodeElement(&lastKey, &start); err != nil {
				return "", err
			}
		case "string":
			var value string
			if err := decoder.DecodeElement(&value, &start); err != nil {
				return "", err
			}
			if lastKey == "VolumeUUID" {
				value = strings.TrimSpace(value)
				if value == "" {
					return "", errors.New("diskutil returned an empty VolumeUUID")
				}
				return value, nil
			}
		}
	}
}
