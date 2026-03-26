package debugdetect

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	protest "github.com/go-delve/delve/pkg/proc/test"
	"github.com/go-delve/delve/service/rpc2"
)

func TestIntegration_NotAttached(t *testing.T) {
	// Build the fixture
	fixturesDir := protest.FindFixturesDir()
	fixtureSrc := filepath.Join(fixturesDir, "debugdetect.go")
	fixtureBin := filepath.Join(t.TempDir(), "debugdetect")
	if runtime.GOOS == "windows" {
		fixtureBin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", fixtureBin, fixtureSrc)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("failed to build fixture: %v\n%s", err, out)
	}

	// Run the fixture (not under debugger)
	cmd = exec.Command(fixtureBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture failed: %v\n%s", err, out)
	}

	output := string(out)
	if !strings.Contains(output, "NOT_ATTACHED") {
		t.Errorf("expected 'NOT_ATTACHED' in output, got: %s", output)
	}
}

func TestIntegration_WaitForDebugger(t *testing.T) {
	// This test verifies that WaitForDebugger() blocks until a debugger
	// attaches and then returns successfully by running the fixture
	// under Delve.
	protest.AllowRecording(t)

	dlvbin := protest.GetDlvBinary(t)
	fixturesDir := protest.FindFixturesDir()
	fixtureSrc := filepath.Join(fixturesDir, "waitfordebugger.go")

	// NOTE(happygo): Use :0 to let the OS pick a free port. happygo runs
	// Delve tests with plain `go test ./...` (concurrent packages), so
	// hard-coded ports cause bind collisions.
	cmd := exec.Command(dlvbin, "debug", fixtureSrc, "--headless", "--continue", "--accept-multiclient", "--listen", "127.0.0.1:0")
	stdout := protest.NewDlvStdout(t, cmd)
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	listenAddr := stdout.ReadPort(t)
	foundOutput := false
	for stdout.Scanner.Scan() {
		line := stdout.Scanner.Text()
		t.Log(line)
		if strings.Contains(line, "DEBUGGER_FOUND") {
			foundOutput = true
			break
		}
	}

	// Clean up - connect and detach
	client := rpc2.NewClient(listenAddr)
	client.Detach(true)
	cmd.Wait()

	if !foundOutput {
		t.Error("expected 'DEBUGGER_FOUND' in output when running under debugger")
	}
}

func TestIntegration_Attached(t *testing.T) {
	// This test verifies that IsDebuggerAttached() returns true when
	// the process is actually running under a debugger by using
	// Delve to debug the fixture source file
	protest.AllowRecording(t)

	dlvbin := protest.GetDlvBinary(t)
	fixturesDir := protest.FindFixturesDir()
	fixtureSrc := filepath.Join(fixturesDir, "debugdetect.go")

	// Run the fixture under dlv debug with --headless --continue
	// This will attach the debugger, compile and run the program
	cmd := exec.Command(dlvbin, "debug", fixtureSrc, "--headless", "--continue", "--accept-multiclient", "--listen", "127.0.0.1:0")
	stdout := protest.NewDlvStdout(t, cmd)
	defer stdout.Close()

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	listenAddr := stdout.ReadPort(t)
	foundOutput := false
	for stdout.Scanner.Scan() {
		line := stdout.Scanner.Text()
		t.Log(line)
		if strings.Contains(line, "ATTACHED") {
			foundOutput = true
			break
		}
	}

	// Clean up - connect and detach
	client := rpc2.NewClient(listenAddr)
	client.Detach(true)
	cmd.Wait()

	if !foundOutput {
		t.Error("expected 'ATTACHED' in output when running under debugger")
	}
}
