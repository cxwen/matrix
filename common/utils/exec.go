package utils

import (
	"bytes"
	"os/exec"
)

func ExecCmd(shell string, cmdStr string) (string, error) {
	cmd := exec.Command(shell, "-c", cmdStr)
	var out bytes.Buffer

	cmd.Stdout = &out
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return out.String(), nil
}
