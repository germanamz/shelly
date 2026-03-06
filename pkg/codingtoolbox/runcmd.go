package codingtoolbox

import (
	"os/exec"
	"strings"
)

// MaxBufferSize is the maximum number of bytes captured from stdout/stderr (1MB).
const MaxBufferSize = 1 << 20

// RunCmd executes cmd with stdout and stderr captured via LimitedBuffer.
// It returns the assembled output (stdout, then stderr separated by a newline
// if both are non-empty) and any error from cmd.Run. On error the caller
// receives both the output and the raw error so it can wrap them as needed.
func RunCmd(cmd *exec.Cmd) (string, error) {
	stdout := NewLimitedBuffer(MaxBufferSize)
	stderr := NewLimitedBuffer(MaxBufferSize)
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()

	var result strings.Builder
	if stdout.Len() > 0 {
		result.WriteString(stdout.String())
	}

	if stderr.Len() > 0 {
		if result.Len() > 0 {
			result.WriteString("\n")
		}

		result.WriteString(stderr.String())
	}

	return result.String(), err
}
