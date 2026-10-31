package gorest

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestAbortStartupClosesServicesAndExits(t *testing.T) {
	const childEnv = "GOREST_ABORT_STARTUP_CHILD"
	if os.Getenv(childEnv) == "1" {
		(&App{}).abortStartup(errors.New("startup failure"))
		return
	}

	command := exec.Command(os.Args[0], "-test.run=^TestAbortStartupClosesServicesAndExits$")
	command.Env = append(os.Environ(), childEnv+"=1")
	output, err := command.CombinedOutput()

	var exitError *exec.ExitError
	if !errors.As(err, &exitError) || exitError.ExitCode() != 1 {
		t.Fatalf("abortStartup() process error = %v, output: %s; want exit status 1", err, output)
	}
	if !strings.Contains(string(output), "application startup aborted: startup failure") {
		t.Fatalf("abortStartup() output = %q, want startup failure", output)
	}
	if !strings.Contains(string(output), "Application closed successfully") {
		t.Fatalf("abortStartup() output = %q, want service shutdown report", output)
	}
}
