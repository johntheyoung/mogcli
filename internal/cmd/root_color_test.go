package cmd

import (
	"io"
	"os"
	"strings"
	"testing"
)

func TestBareInvocationShowsHelp(t *testing.T) {
	stdout, stderr, err := captureExecuteOutput(t, []string{})
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	if strings.TrimSpace(stderr) != "" {
		t.Fatalf("expected no stderr on bare invocation, got:\n%s", stderr)
	}
	if !strings.Contains(stdout, "USAGE") {
		t.Fatalf("expected USAGE section, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "COMMANDS") {
		t.Fatalf("expected COMMANDS section, got:\n%s", stdout)
	}
}

func TestHelpDoesNotExposeColorFlag(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"--help"})
	if err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	if strings.Contains(stdout, "--color") {
		t.Fatalf("help output must not contain --color:\n%s", stdout)
	}
}

func TestRootHelpPolishSections(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"--help"})
	if err != nil {
		t.Fatalf("Execute(--help) failed: %v", err)
	}

	if strings.Contains(stdout, "Build:") {
		t.Fatalf("help output must not contain build metadata:\n%s", stdout)
	}
	if strings.Contains(stdout, "Config:") {
		t.Fatalf("help output must not contain config diagnostics:\n%s", stdout)
	}
	if !strings.Contains(stdout, "LEARN MORE") {
		t.Fatalf("expected LEARN MORE section, got:\n%s", stdout)
	}
}

func TestSubcommandHelpShowsInheritedFlagsAndExamples(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"mail", "list", "--help"})
	if err != nil {
		t.Fatalf("Execute(mail list --help) failed: %v", err)
	}
	if !strings.Contains(stdout, "INHERITED FLAGS") {
		t.Fatalf("expected INHERITED FLAGS section, got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "EXAMPLES") {
		t.Fatalf("expected EXAMPLES section, got:\n%s", stdout)
	}
}

func TestMailFoldersHelpDocumentsHiddenAndPagingBehavior(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"mail", "folders", "--help"})
	if err != nil {
		t.Fatalf("Execute(mail folders --help) failed: %v", err)
	}
	for _, expected := range []string{
		"--include-hidden",
		"--page",
		"--max",
		"opaque Graph next-page URL",
		"EXAMPLES",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected mail folders help to contain %q, got:\n%s", expected, stdout)
		}
	}
}

func TestAuthHelpIncludesAppSubcommand(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"auth", "--help"})
	if err != nil {
		t.Fatalf("Execute(auth --help) failed: %v", err)
	}
	if !strings.Contains(stdout, "app") {
		t.Fatalf("expected auth help to include app subcommand, got:\n%s", stdout)
	}
}

func TestAuthHelpIncludesUpdateSubcommand(t *testing.T) {
	stdout, _, err := captureExecuteOutput(t, []string{"auth", "--help"})
	if err != nil {
		t.Fatalf("Execute(auth --help) failed: %v", err)
	}
	if !strings.Contains(stdout, "update") {
		t.Fatalf("expected auth help to include update subcommand, got:\n%s", stdout)
	}
}

func TestRemovedColorFlagIsUnknownUsageError(t *testing.T) {
	_, stderr, err := captureExecuteOutput(t, []string{"--color", "always", "version"})
	if err == nil {
		t.Fatal("expected usage error for removed --color flag")
	}
	if code := ExitCode(err); code != 2 {
		t.Fatalf("expected usage exit code 2, got %d (err=%v)", code, err)
	}
	if !strings.Contains(stderr, "unknown flag --color") {
		t.Fatalf("expected unknown-flag error for --color, got:\n%s", stderr)
	}
	if !strings.HasPrefix(strings.TrimSpace(stderr), "Error:") {
		t.Fatalf("expected Error: prefix, got:\n%s", stderr)
	}
}

func TestMOGColorEnvDoesNotAffectVersionExecution(t *testing.T) {
	t.Setenv("MOG_COLOR", "always")

	stdout, stderr, err := captureExecuteOutput(t, []string{"version"})
	if err != nil {
		t.Fatalf("Execute(version) failed: %v\nstderr:\n%s", err, stderr)
	}
	if got, want := strings.TrimSpace(stdout), VersionString(); got != want {
		t.Fatalf("unexpected version output: got %q want %q", got, want)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "mog version ") {
		t.Fatalf("version output should use gh-style prefix, got %q", strings.TrimSpace(stdout))
	}
}

func captureExecuteOutput(t *testing.T, args []string) (stdout string, stderr string, runErr error) {
	t.Helper()

	origStdout := os.Stdout
	origStderr := os.Stderr

	rOut, wOut, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	rErr, wErr, err := os.Pipe()
	if err != nil {
		_ = rOut.Close()
		_ = wOut.Close()
		t.Fatalf("create stderr pipe: %v", err)
	}

	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		os.Stdout = origStdout
		os.Stderr = origStderr
	}()

	runErr = Execute(args)

	if err := wOut.Close(); err != nil {
		t.Fatalf("close stdout writer: %v", err)
	}
	if err := wErr.Close(); err != nil {
		t.Fatalf("close stderr writer: %v", err)
	}

	outBytes, err := io.ReadAll(rOut)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	errBytes, err := io.ReadAll(rErr)
	if err != nil {
		t.Fatalf("read stderr: %v", err)
	}

	if err := rOut.Close(); err != nil {
		t.Fatalf("close stdout reader: %v", err)
	}
	if err := rErr.Close(); err != nil {
		t.Fatalf("close stderr reader: %v", err)
	}

	return string(outBytes), string(errBytes), runErr
}
