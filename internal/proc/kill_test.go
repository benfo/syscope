package proc

import (
	"errors"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// sleeper starts a long-running child process with no window.
func sleeper(t *testing.T) *exec.Cmd {
	t.Helper()
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("ping", "-n", "60", "127.0.0.1")
	} else {
		cmd = exec.Command("sleep", "60")
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	return cmd
}

func waitExit(t *testing.T, cmd *exec.Cmd) {
	t.Helper()
	done := make(chan struct{})
	go func() { _ = cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("process did not exit")
	}
}

func TestForceKill(t *testing.T) {
	cmd := sleeper(t)
	if err := Kill(int32(cmd.Process.Pid), true); err != nil {
		t.Fatal(err)
	}
	waitExit(t, cmd)
}

func TestGracefulKill(t *testing.T) {
	cmd := sleeper(t)
	err := Kill(int32(cmd.Process.Pid), false)
	if runtime.GOOS == "windows" {
		if !errors.Is(err, ErrNoWindow) {
			t.Fatalf("windowless process: got %v, want ErrNoWindow", err)
		}
		return
	}
	if err != nil {
		t.Fatal(err)
	}
	waitExit(t, cmd)
}

func TestKillMissing(t *testing.T) {
	// PIDs are multiples of 4 on Windows; an odd huge PID never exists.
	for _, force := range []bool{false, true} {
		if err := Kill(0x7ffffff1, force); !errors.Is(err, ErrNotFound) {
			t.Errorf("force=%v: got %v, want ErrNotFound", force, err)
		}
	}
}
