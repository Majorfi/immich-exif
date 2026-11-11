//go:build darwin

package process

import (
	"os/exec"
	"strings"
)

func hasACL(path string) bool {
	output, err := exec.Command("/bin/ls", "-led", path).Output()
	if err != nil {
		return true
	}
	return strings.Count(strings.TrimSpace(string(output)), "\n") > 0
}
