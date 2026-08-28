package backup

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

type LaunchctlController struct{}

func (LaunchctlController) Kickstart(ctx context.Context, service string) (string, error) {
	labels := map[string]string{
		"backup":        "com.apofocus.backup",
		"backup-verify": "com.apofocus.backup-verify",
	}
	label, ok := labels[service]
	if !ok {
		return "", errors.New("backup service must be backup or backup-verify")
	}
	if runtime.GOOS != "darwin" {
		return label, errors.New("scheduled backup control is available only on macOS")
	}
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	command := exec.CommandContext(ctx, "/bin/launchctl", "kickstart", target)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return label, fmt.Errorf("start %s: %s", label, detail)
	}
	return label, nil
}
