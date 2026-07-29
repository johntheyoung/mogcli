package cmd

import "testing"

func TestDryRunFlagsParse(t *testing.T) {
	testCases := []struct {
		name string
		args []string
		get  func(*CLI) bool
	}{
		{
			name: "mail send --dry-run",
			args: []string{"mail", "send", "--to", "dev@example.com", "--subject", "Test", "--body", "Body", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Mail.Send.DryRun },
		},
		{
			name: "mail archive --dry-run",
			args: []string{"mail", "archive", "message-id", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Mail.Archive.DryRun },
		},
		{
			name: "mail move --dry-run",
			args: []string{"mail", "move", "message-id", "--folder", "archive", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Mail.Move.DryRun },
		},
		{
			name: "mail mark-read --dry-run",
			args: []string{"mail", "mark-read", "message-id", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Mail.MarkRead.DryRun },
		},
		{
			name: "calendar create --dry-run",
			args: []string{
				"calendar", "create",
				"--subject", "Planning",
				"--start", "2026-02-13T16:00:00Z",
				"--end", "2026-02-13T16:30:00Z",
				"--dry-run",
			},
			get: func(cli *CLI) bool { return cli.Calendar.Create.DryRun },
		},
		{
			name: "calendar update --dry-run",
			args: []string{"calendar", "update", "event-id", "--subject", "Updated", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Calendar.Update.DryRun },
		},
		{
			name: "calendar delete --dry-run",
			args: []string{"calendar", "delete", "event-id", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.Calendar.Delete.DryRun },
		},
		{
			name: "onedrive put --dry-run",
			args: []string{"onedrive", "put", "./local.txt", "--path", "/remote.txt", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.OneDrive.Put.DryRun },
		},
		{
			name: "onedrive mkdir --dry-run",
			args: []string{"onedrive", "mkdir", "--path", "/reports", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.OneDrive.Mkdir.DryRun },
		},
		{
			name: "onedrive rm --dry-run",
			args: []string{"onedrive", "rm", "--path", "/reports/old.txt", "--dry-run"},
			get:  func(cli *CLI) bool { return cli.OneDrive.RM.DryRun },
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parser, cli, err := newParser("test")
			if err != nil {
				t.Fatalf("newParser failed: %v", err)
			}
			if _, err := parser.Parse(tc.args); err != nil {
				t.Fatalf("parse failed: %v", err)
			}
			if !tc.get(cli) {
				t.Fatalf("expected --dry-run to set flag true for args: %#v", tc.args)
			}
		})
	}
}
