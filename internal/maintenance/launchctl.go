package maintenance

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

func (LaunchctlController) Restart(ctx context.Context, service string) (string, error) {
	labels := map[string]string{
		"postgres":  "com.apofocus.postgres",
		"web":       "com.apofocus.web",
		"embedding": "com.apofocus.embedding",
		"worker":    "com.apofocus.worker",
	}
	label, ok := labels[service]
	if !ok {
		return "", errors.New("service must be postgres, web, embedding, or worker")
	}
	if runtime.GOOS != "darwin" {
		return label, errors.New("managed service repair is available only on macOS")
	}
	target := fmt.Sprintf("gui/%d/%s", os.Getuid(), label)
	command := exec.CommandContext(ctx, "/bin/launchctl", "kickstart", "-k", target)
	output, err := command.CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(output))
		if detail == "" {
			detail = err.Error()
		}
		return label, fmt.Errorf("restart %s: %s", label, detail)
	}
	return label, nil
}
