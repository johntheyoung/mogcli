package cmd

import (
	"strings"
	"testing"

	"github.com/alecthomas/kong"
)

func TestCommandActionCanonicalizesPositionalResources(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "mail get",
			args: []string{"mail", "get", "message-id"},
			want: "mail.get",
		},
		{
			name: "mail archive",
			args: []string{"mail", "archive", "message/id", "--dry-run"},
			want: "mail.archive",
		},
		{
			name: "mail move",
			args: []string{"mail", "move", "message/id", "--folder", "archive", "--dry-run"},
			want: "mail.move",
		},
		{
			name: "mail mark-read",
			args: []string{"mail", "mark-read", "message/id", "--dry-run"},
			want: "mail.mark-read",
		},
		{
			name: "calendar get",
			args: []string{"calendar", "get", "event-id"},
			want: "calendar.get",
		},
		{
			name: "contacts get",
			args: []string{"contacts", "get", "contact-id"},
			want: "contacts.get",
		},
		{
			name: "groups members",
			args: []string{"groups", "members", "group-id"},
			want: "groups.members",
		},
		{
			name: "onedrive get",
			args: []string{"onedrive", "get", "/Reports/report.pdf"},
			want: "onedrive.get",
		},
		{
			name: "positional write stays distinct",
			args: []string{"calendar", "delete", "event-id", "--dry-run"},
			want: "calendar.delete",
		},
		{
			name: "mail list stays distinct",
			args: []string{"mail", "list"},
			want: "mail.list",
		},
		{
			name: "mail folders stays distinct",
			args: []string{"mail", "folders"},
			want: "mail.folders",
		},
		{
			name: "teams write stays distinct",
			args: []string{
				"teams", "channel-send",
				"--team", "team-id",
				"--channel", "channel-id",
				"--body", "Hello",
			},
			want: "teams.channel-send",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parser, _, err := newParser("test")
			if err != nil {
				t.Fatalf("newParser failed: %v", err)
			}
			kctx, err := parser.Parse(tc.args)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if got := commandAction(kctx); got != tc.want {
				t.Fatalf("expected action %q, got %q", tc.want, got)
			}
		})
	}
}

func TestEnableActionsAuthorizesCanonicalPositionalActionOnly(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		enabled string
		wantErr bool
	}{
		{
			name:    "mail get is authorized",
			args:    []string{"mail", "get", "message-id"},
			enabled: "mail.get",
		},
		{
			name:    "calendar get is authorized",
			args:    []string{"calendar", "get", "event-id"},
			enabled: "calendar.get",
		},
		{
			name:    "mail get does not authorize list",
			args:    []string{"mail", "list"},
			enabled: "mail.get",
			wantErr: true,
		},
		{
			name:    "mail list does not authorize folders",
			args:    []string{"mail", "folders"},
			enabled: "mail.list",
			wantErr: true,
		},
		{
			name: "teams list does not authorize channel send",
			args: []string{
				"teams", "channel-send",
				"--team", "team-id",
				"--channel", "channel-id",
				"--body", "Hello",
			},
			enabled: "teams.list",
			wantErr: true,
		},
		{
			name:    "absent allowlist remains unrestricted",
			args:    []string{"mail", "folders"},
			enabled: "",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parser, _, err := newParser("test")
			if err != nil {
				t.Fatalf("newParser failed: %v", err)
			}
			kctx, err := parser.Parse(tc.args)
			if err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			err = enforceEnabledActions(kctx, tc.enabled, false)
			if tc.wantErr && err == nil {
				t.Fatal("expected action to be blocked")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected action to be allowed: %v", err)
			}
		})
	}
}

func TestMailMutationActionAllowlistsAuthorizeOnlyTheirOwnCommand(t *testing.T) {
	commands := map[string][]string{
		"mail.archive":   {"mail", "archive", "message-id", "--dry-run"},
		"mail.move":      {"mail", "move", "message-id", "--folder", "archive", "--dry-run"},
		"mail.mark-read": {"mail", "mark-read", "message-id", "--dry-run"},
	}

	for enabled, enabledArgs := range commands {
		enabled := enabled
		enabledArgs := enabledArgs
		t.Run(enabled, func(t *testing.T) {
			for action, args := range commands {
				kctx := parseGuardTestCommand(t, args)
				err := enforceEnabledActions(kctx, enabled, true)
				if action == enabled && err != nil {
					t.Fatalf("%s should authorize itself: %v", enabled, err)
				}
				if action != enabled && err == nil {
					t.Fatalf("%s must not authorize %s", enabled, action)
				}
			}

			kctx := parseGuardTestCommand(t, enabledArgs)
			if err := enforceEnabledActions(kctx, "", true); err == nil {
				t.Fatalf("%s must fail closed without an action allowlist", enabled)
			}
		})
	}
}

func TestAutomationGuardsRemainUnrestrictedWithoutManagedSignal(t *testing.T) {
	kctx := parseGuardTestCommand(t, []string{"mail", "get", "message-id"})

	if err := enforceEnabledCommands(kctx, "", false); err != nil {
		t.Fatalf("interactive command guard should be unrestricted: %v", err)
	}
	if err := enforceEnabledActions(kctx, "", false); err != nil {
		t.Fatalf("interactive action guard should be unrestricted: %v", err)
	}
}

func TestAutomationGuardsFailClosedInManagedModeWithMissingOrEmptyAllowlists(t *testing.T) {
	kctx := parseGuardTestCommand(t, []string{"mail", "get", "message-id"})

	for _, enabled := range []string{"", "  ", ", ,"} {
		if err := enforceEnabledCommands(kctx, enabled, true); err == nil || !strings.Contains(err.Error(), "MOG_ENABLE_COMMANDS") {
			t.Fatalf("managed command guard should reject %q with guidance, got %v", enabled, err)
		}
		if err := enforceEnabledActions(kctx, enabled, true); err == nil || !strings.Contains(err.Error(), "MOG_ENABLE_ACTIONS") {
			t.Fatalf("managed action guard should reject %q with guidance, got %v", enabled, err)
		}
	}
}

func TestAutomationGuardsHonorExplicitManagedAllowlists(t *testing.T) {
	kctx := parseGuardTestCommand(t, []string{"mail", "get", "message-id"})

	if err := enforceEnabledCommands(kctx, "mail", true); err != nil {
		t.Fatalf("managed command allowlist should permit mail: %v", err)
	}
	if err := enforceEnabledActions(kctx, "mail.get", true); err != nil {
		t.Fatalf("managed action allowlist should permit normalized positional action: %v", err)
	}
}

func TestManagedAutomationModeUsesOnlyExplicitSignal(t *testing.T) {
	t.Setenv(managedAutomationEnv, "")
	managed, err := managedAutomationMode(false)
	if err != nil || managed {
		t.Fatalf("empty signal should leave interactive mode unrestricted: managed=%v err=%v", managed, err)
	}

	t.Setenv(managedAutomationEnv, "true")
	managed, err = managedAutomationMode(false)
	if err != nil || !managed {
		t.Fatalf("explicit signal should enable managed mode: managed=%v err=%v", managed, err)
	}

	t.Setenv(managedAutomationEnv, "not-a-boolean")
	if _, err := managedAutomationMode(false); err == nil {
		t.Fatal("invalid explicit managed automation signal should fail")
	}
}

func TestExecuteManagedAutomationRequiresBothAllowlists(t *testing.T) {
	t.Setenv(managedAutomationEnv, "true")
	t.Setenv("MOG_ENABLE_COMMANDS", "")
	t.Setenv("MOG_ENABLE_ACTIONS", "")

	_, stderr, err := captureExecuteOutput(t, []string{"version"})
	if err == nil || !strings.Contains(stderr, "MOG_ENABLE_COMMANDS") {
		t.Fatalf("managed execution should fail closed on missing command allowlist: err=%v stderr=%q", err, stderr)
	}

	t.Setenv("MOG_ENABLE_COMMANDS", "version")
	_, stderr, err = captureExecuteOutput(t, []string{"version"})
	if err == nil || !strings.Contains(stderr, "MOG_ENABLE_ACTIONS") {
		t.Fatalf("managed execution should fail closed on missing action allowlist: err=%v stderr=%q", err, stderr)
	}

	t.Setenv("MOG_ENABLE_ACTIONS", "version")
	stdout, stderr, err := captureExecuteOutput(t, []string{"version"})
	if err != nil {
		t.Fatalf("managed execution with explicit allowlists failed: %v stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, "mog version") {
		t.Fatalf("unexpected version output: %q", stdout)
	}
}

func parseGuardTestCommand(t *testing.T, args []string) *kong.Context {
	t.Helper()
	parser, _, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}
	kctx, err := parser.Parse(args)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	return kctx
}
