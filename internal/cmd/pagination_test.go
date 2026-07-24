package cmd

import (
	"errors"
	"testing"
)

func TestNormalizePageToken(t *testing.T) {
	valid := "https://graph.microsoft.com/v1.0/me/messages?$skiptoken=abc"
	validGov := "https://graph.microsoft.us/v1.0/me/messages?$skiptoken=abc"

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "empty", input: "", want: ""},
		{name: "trim empty", input: "   ", want: ""},
		{name: "absolute url", input: valid, want: valid},
		{name: "absolute gov url", input: validGov, want: validGov},
		{name: "relative url", input: "/me/messages?$skiptoken=abc", wantErr: true},
		{name: "unsupported scheme", input: "ftp://graph.microsoft.com/v1.0/me/messages", wantErr: true},
		{name: "untrusted host", input: "https://evil.example/v1.0/me/messages?$skiptoken=abc", wantErr: true},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizePageToken(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error")
				}
				var exitErr *ExitError
				if !errors.As(err, &exitErr) || exitErr.Code != 2 {
					t.Fatalf("expected usage ExitError code 2, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("expected %q, got %q", tc.want, got)
			}
		})
	}
}

func TestPaginationFlagsAcceptPageAndNextToken(t *testing.T) {
	const pageToken = "https://graph.microsoft.com/v1.0/me/messages?$skiptoken=abc"

	tests := []struct {
		name string
		args []string
		get  func(*CLI) string
	}{
		{
			name: "onedrive ls --page",
			args: []string{"onedrive", "ls", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.OneDrive.LS.Page },
		},
		{
			name: "onedrive ls --next-token",
			args: []string{"onedrive", "ls", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.OneDrive.LS.Page },
		},
		{
			name: "groups list --page",
			args: []string{"groups", "list", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Groups.List.Page },
		},
		{
			name: "groups list --next-token",
			args: []string{"groups", "list", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Groups.List.Page },
		},
		{
			name: "groups members --page",
			args: []string{"groups", "members", "group-id", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Groups.Members.Page },
		},
		{
			name: "groups members --next-token",
			args: []string{"groups", "members", "group-id", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Groups.Members.Page },
		},
		{
			name: "mail list --page",
			args: []string{"mail", "list", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Mail.List.Page },
		},
		{
			name: "mail list --next-token",
			args: []string{"mail", "list", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Mail.List.Page },
		},
		{
			name: "mail folders --page",
			args: []string{"mail", "folders", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Mail.Folders.Page },
		},
		{
			name: "mail folders --next-token",
			args: []string{"mail", "folders", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Mail.Folders.Page },
		},
		{
			name: "calendar list --page",
			args: []string{"calendar", "list", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Calendar.List.Page },
		},
		{
			name: "calendar list --next-token",
			args: []string{"calendar", "list", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Calendar.List.Page },
		},
		{
			name: "contacts list --page",
			args: []string{"contacts", "list", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Contacts.List.Page },
		},
		{
			name: "contacts list --next-token",
			args: []string{"contacts", "list", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Contacts.List.Page },
		},
		{
			name: "tasks list --page",
			args: []string{"tasks", "list", "--list", "list-id", "--page", pageToken},
			get:  func(cli *CLI) string { return cli.Tasks.List.Page },
		},
		{
			name: "tasks list --next-token",
			args: []string{"tasks", "list", "--list", "list-id", "--next-token", pageToken},
			get:  func(cli *CLI) string { return cli.Tasks.List.Page },
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			parser, cli, err := newParser("test")
			if err != nil {
				t.Fatalf("newParser failed: %v", err)
			}
			if _, err := parser.Parse(tc.args); err != nil {
				t.Fatalf("parse failed: %v", err)
			}

			if got := tc.get(cli); got != pageToken {
				t.Fatalf("expected page token %q, got %q", pageToken, got)
			}
		})
	}
}
