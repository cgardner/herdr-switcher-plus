//go:build e2e

// Package e2e runs the built binary the way Herdr runs it: as a process, with
// the Herdr environment set, a Herdr CLI on HERDR_BIN_PATH and a Herdr socket
// on HERDR_SOCKET_PATH. The unit tests replace every seam in the program.
// These tests replace none, so they prove that the seams join up.
//
// Both Herdr stand-ins live in this package. The CLI is testdata/herdr, a
// script that records its calls. The socket is a unix socket server in
// socket_test.go that records its requests. No test can touch a real session.
//
// Run them with `make e2e`. E2E_BIN names a binary to test in place of a fresh
// build, for a check of a release asset.
package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// binary is the program under test, built once by TestMain.
var binary string

// repoRoot is the module root, where the demo fixture script lives.
var repoRoot string

func TestMain(m *testing.M) {
	os.Exit(run(m))
}

func run(m *testing.M) int {
	_, file, _, _ := runtime.Caller(0)
	repoRoot = filepath.Dir(filepath.Dir(file))

	if binary = os.Getenv("E2E_BIN"); binary != "" {
		return m.Run()
	}
	dir, err := os.MkdirTemp("", "e2e-bin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "e2e:", err)
		return 1
	}
	defer os.RemoveAll(dir)

	binary = filepath.Join(dir, "herdr-switcher-plus")
	build := exec.Command("go", "build",
		"-ldflags", "-X github.com/cgardner/herdr-switcher-plus/internal/cli.version=e2e",
		"-o", binary, ".")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "e2e: build failed: %v\n%s", err, out)
		return 1
	}
	return m.Run()
}

// env is one throwaway Herdr world: a fixture of invented sessions, a fake CLI
// and a fake socket, each in a directory of its own.
type env struct {
	t       *testing.T
	dir     string // the fixture root
	config  string // HERDR_PLUGIN_CONFIG_DIR
	state   string // HERDR_PLUGIN_STATE_DIR
	home    string
	socket  *socketServer
	extra   []string // environment a test adds
	started time.Time
}

// newEnv builds the fixture from scripts/demo_fixture.py, the one source of
// invented sessions in this repository, and starts the fake socket.
func newEnv(t *testing.T) *env {
	t.Helper()
	// Each test owns its world, so they can share the machine.
	t.Parallel()
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 builds the fixture and is not installed")
	}
	dir := t.TempDir()
	if out, err := exec.Command("python3", "-I", filepath.Join(repoRoot, "scripts", "demo_fixture.py"), dir).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v\n%s", err, out)
	}
	fake, err := os.ReadFile(filepath.Join(repoRoot, "e2e", "testdata", "herdr"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "herdr"), fake, 0o755); err != nil {
		t.Fatal(err)
	}
	return &env{
		t: t, dir: dir,
		home:    filepath.Join(dir, "home"),
		config:  filepath.Join(dir, "config"),
		state:   filepath.Join(dir, "state"),
		socket:  startSocket(t),
		started: time.Now(),
	}
}

// with adds environment variables, as KEY=value, for the next runs.
func (e *env) with(kv ...string) *env {
	e.extra = append(e.extra, kv...)
	return e
}

// environ is the whole environment of the program. It starts empty on purpose:
// a variable inherited from the Herdr pane that runs the tests would point the
// program at a real session.
func (e *env) environ() []string {
	return append([]string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + e.home,
		"LANG=en_US.UTF-8",
		"TERM=xterm-256color",
		"HERDR_BIN_PATH=" + filepath.Join(e.dir, "herdr"),
		"HERDR_SOCKET_PATH=" + e.socket.path,
		"HERDR_PLUGIN_CONFIG_DIR=" + e.config,
		"HERDR_PLUGIN_STATE_DIR=" + e.state,
	}, e.extra...)
}

// result is a finished run.
type result struct {
	stdout, stderr string
	code           int
}

// run starts the binary without a terminal and waits for it.
func (e *env) run(args ...string) result {
	e.t.Helper()
	cmd := exec.Command(binary, args...)
	cmd.Env = e.environ()
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	done := make(chan error, 1)
	if err := cmd.Start(); err != nil {
		e.t.Fatal(err)
	}
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		code := 0
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else if err != nil {
			e.t.Fatal(err)
		}
		return result{out.String(), errb.String(), code}
	case <-time.After(20 * time.Second):
		_ = cmd.Process.Kill()
		e.t.Fatalf("%v did not exit\nstdout:\n%s\nstderr:\n%s", args, out.String(), errb.String())
		return result{}
	}
}

// lines is the output split by line, without the final empty one.
func (r result) lines() []string {
	return strings.Split(strings.TrimRight(r.stdout, "\n"), "\n")
}

// writeConfig puts text in the plugin's config.toml.
func (e *env) writeConfig(text string) {
	e.t.Helper()
	if err := os.MkdirAll(e.config, 0o755); err != nil {
		e.t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(e.config, "config.toml"), []byte(text), 0o644); err != nil {
		e.t.Fatal(err)
	}
}

// calls is every Herdr CLI call so far, each as its argument list.
func (e *env) calls() [][]string {
	b, err := os.ReadFile(filepath.Join(e.dir, "calls.log"))
	if err != nil {
		return nil
	}
	var all [][]string
	for _, line := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
		all = append(all, strings.Split(strings.TrimSuffix(line, "\t"), "\t"))
	}
	return all
}

// called reports whether a CLI call began with these arguments.
func (e *env) called(prefix ...string) bool {
	return len(e.callsWith(prefix...)) > 0
}

// callsWith is the calls that began with these arguments.
func (e *env) callsWith(prefix ...string) [][]string {
	var out [][]string
	for _, c := range e.calls() {
		if len(c) >= len(prefix) && strings.Join(c[:len(prefix)], "\t") == strings.Join(prefix, "\t") {
			out = append(out, c)
		}
	}
	return out
}
