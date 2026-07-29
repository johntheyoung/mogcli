package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	teamsvc "github.com/jaredpalmer/mogcli/internal/services/teams"
)

func TestTeamsChannelSendDryRunFlagParses(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"teams", "channel-send",
		"--team", "team-id",
		"--channel", "channel-id",
		"--body", "Hello",
		"--dry-run",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !cli.Teams.ChannelSend.DryRun {
		t.Fatal("expected --dry-run to set Teams channel-send dry-run flag")
	}
}

func TestTeamsChatMembersFlagParses(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{"teams", "chat-members", "--chat", "chat-id", "--max", "5"}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if cli.Teams.ChatMembers.Chat != "chat-id" {
		t.Fatalf("unexpected chat: %q", cli.Teams.ChatMembers.Chat)
	}
	if cli.Teams.ChatMembers.Max != 5 {
		t.Fatalf("unexpected max: %d", cli.Teams.ChatMembers.Max)
	}
}

func TestTeamsChatSendDryRunFlagParses(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"teams", "chat-send",
		"--chat", "chat-id",
		"--body", "Hello",
		"--dry-run",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !cli.Teams.ChatSend.DryRun {
		t.Fatal("expected --dry-run to set Teams chat-send dry-run flag")
	}
}

func TestTeamsChatSendMentionFlagParses(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"teams", "chat-send",
		"--chat", "chat-id",
		"--body", "Hello",
		"--mention", "Jane Doe:aad-user-1",
		"--mention", "John Smith:aad-user-2",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	want := []string{"Jane Doe:aad-user-1", "John Smith:aad-user-2"}
	if !reflect.DeepEqual(cli.Teams.ChatSend.Mention, want) {
		t.Fatalf("unexpected mentions: %#v", cli.Teams.ChatSend.Mention)
	}
}

func TestTeamsChatSendDryRunPayloadIncludesMentionMetadata(t *testing.T) {
	mentions := []teamsvc.ChatMention{
		{DisplayName: "Jane Doe", UserID: "aad-user-1"},
		{DisplayName: "John Smith", UserID: "aad-user-2"},
	}

	got := teamsChatSendDryRunPayload("chat-id", "Hello", "text", mentions)
	if got["contentType"] != "html" {
		t.Fatalf("expected mentions to force dry-run contentType html, got %#v", got["contentType"])
	}
	if got["mention_count"] != 2 {
		t.Fatalf("unexpected mention_count: %#v", got["mention_count"])
	}
	wantNames := []string{"Jane Doe", "John Smith"}
	if !reflect.DeepEqual(got["mention_display_names"], wantNames) {
		t.Fatalf("unexpected mention_display_names: %#v", got["mention_display_names"])
	}
	if _, ok := got["mention_ids"]; ok {
		t.Fatalf("dry-run metadata must not expose aad object ids: %#v", got["mention_ids"])
	}
}

func TestTeamsChatSendDryRunPayloadNoMentionShapeUnchanged(t *testing.T) {
	got := teamsChatSendDryRunPayload("chat-id", "Hello", "text", nil)

	if got["contentType"] != "text" {
		t.Fatalf("unexpected contentType: %#v", got["contentType"])
	}
	if _, ok := got["mention_count"]; ok {
		t.Fatalf("expected no mention_count without mentions: %#v", got)
	}
	if _, ok := got["mention_display_names"]; ok {
		t.Fatalf("expected no mention_display_names without mentions: %#v", got)
	}
}

func TestTeamsDMSendDryRunFlagParses(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"teams", "dm-send",
		"--to", "target@contoso.com",
		"--body", "Hello",
		"--dry-run",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if !cli.Teams.DMSend.DryRun {
		t.Fatal("expected --dry-run to set Teams dm-send dry-run flag")
	}
}

func TestEnableActionsAllowsSpecificSubcommand(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	kctx, err := parser.Parse([]string{"teams", "list"})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	cli.EnableActions = "teams.list"

	if err := enforceEnabledActions(kctx, cli.EnableActions, false); err != nil {
		t.Fatalf("expected teams.list to be allowed: %v", err)
	}
}

func TestEnableActionsAllowsDMSendSubcommand(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	kctx, err := parser.Parse([]string{
		"teams", "dm-send",
		"--to", "target@contoso.com",
		"--body", "Hello",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	cli.EnableActions = "teams.dm-send"

	if err := enforceEnabledActions(kctx, cli.EnableActions, false); err != nil {
		t.Fatalf("expected teams.dm-send to be allowed: %v", err)
	}
}

func TestEnableActionsBlocksUnlistedSubcommand(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	kctx, err := parser.Parse([]string{
		"teams", "channel-send",
		"--team", "team-id",
		"--channel", "channel-id",
		"--body", "Hello",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	cli.EnableActions = "teams.list,teams.channels"

	if err := enforceEnabledActions(kctx, cli.EnableActions, false); err == nil {
		t.Fatal("expected teams.channel-send to be blocked")
	}
}

func TestTeamsChatFileSendRegistrationAndFlags(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}
	kctx, err := parser.Parse([]string{
		"teams", "chat-file-send",
		"--chat", "chat-id",
		"--file", localPath,
		"--name", "safe report.txt",
		"--body", "Please review",
		"--dry-run",
	})
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if got := commandAction(kctx); got != "teams.chat-file-send" {
		t.Fatalf("unexpected action: %q", got)
	}
	command := cli.Teams.ChatFileSend
	if command.Chat != "chat-id" ||
		command.File != localPath ||
		command.Name != "safe report.txt" ||
		command.Body != "Please review" ||
		!command.DryRun {
		t.Fatalf("unexpected parsed command: %#v", command)
	}
}

func TestTeamsChatFileSendHelpDocumentsSafeShape(t *testing.T) {
	stdout, stderr, err := captureExecuteOutput(t, []string{"teams", "chat-file-send", "--help"})
	if err != nil {
		t.Fatalf("help failed: %v stderr=%q", err, stderr)
	}
	for _, expected := range []string{
		"--chat",
		"--file",
		"--name",
		"--body",
		"--dry-run",
		"one-on-one",
		"symlinks are rejected",
		"without authentication or Graph",
		"calls",
	} {
		if !strings.Contains(stdout, expected) {
			t.Fatalf("expected help to contain %q, got:\n%s", expected, stdout)
		}
	}
}

func TestTeamsChatFileSendDryRunNeedsNoProfileTokenOrNetwork(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MOG_PROFILE", "profile-that-must-not-be-resolved-for-dry-run")
	t.Setenv("MOG_ENABLE_COMMANDS", "")
	t.Setenv("MOG_ENABLE_ACTIONS", "")

	stdout, stderr, err := captureExecuteOutput(t, []string{
		"teams", "chat-file-send",
		"--chat", "chat-id",
		"--file", localPath,
		"--dry-run",
		"--json",
	})
	if err != nil {
		t.Fatalf("dry run failed without a usable profile: %v stderr=%q", err, stderr)
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(stdout), &payload); err != nil {
		t.Fatalf("decode dry-run JSON: %v\n%s", err, stdout)
	}
	if payload["action"] != "teams.chat-file-send" || payload["chat_id"] != "chat-id" || payload["filename"] != "report.txt" {
		t.Fatalf("unexpected dry-run output: %#v", payload)
	}
	if payload["conflict_policy"] != "fail" || payload["sha256"] == "" || payload["bytes"] != float64(5) {
		t.Fatalf("missing dry-run safety metadata: %#v", payload)
	}
	if strings.Contains(stdout, filepath.Dir(localPath)) || strings.Contains(stdout, "hello") {
		t.Fatalf("dry-run output exposed a local parent path or content: %s", stdout)
	}
}

func TestTeamsChatFileSendGuardIsSeparateFromExistingWrites(t *testing.T) {
	localPath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	newAction := parseGuardTestCommand(t, []string{
		"teams", "chat-file-send",
		"--chat", "chat-id",
		"--file", localPath,
		"--dry-run",
	})

	if err := enforceEnabledActions(newAction, "teams.chat-file-send", true); err != nil {
		t.Fatalf("dedicated action should authorize chat-file-send: %v", err)
	}
	for _, existing := range []string{"teams.chat-send", "teams.dm-send", "onedrive.put"} {
		if err := enforceEnabledActions(newAction, existing, true); err == nil {
			t.Fatalf("%s must not authorize teams.chat-file-send", existing)
		}
	}

	oldAction := parseGuardTestCommand(t, []string{
		"teams", "chat-send",
		"--chat", "chat-id",
		"--body", "hello",
		"--dry-run",
	})
	if err := enforceEnabledActions(oldAction, "teams.chat-file-send", true); err == nil {
		t.Fatal("teams.chat-file-send must not authorize teams.chat-send")
	}
}
