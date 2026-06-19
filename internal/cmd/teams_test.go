package cmd

import "testing"

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

	if err := enforceEnabledActions(kctx, cli.EnableActions); err != nil {
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

	if err := enforceEnabledActions(kctx, cli.EnableActions); err != nil {
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

	if err := enforceEnabledActions(kctx, cli.EnableActions); err == nil {
		t.Fatal("expected teams.channel-send to be blocked")
	}
}
