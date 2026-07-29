package cmd

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestMailReadFlagsParse(t *testing.T) {
	t.Run("folder-scoped list", func(t *testing.T) {
		parser, cli, err := newParser("test")
		if err != nil {
			t.Fatalf("newParser failed: %v", err)
		}

		args := []string{
			"mail", "list",
			"--folder", "inbox",
			"--max", "50",
			"--query", "isRead:false",
		}
		if _, err := parser.Parse(args); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		if cli.Mail.List.Folder != "inbox" {
			t.Fatalf("unexpected folder: %q", cli.Mail.List.Folder)
		}
		if cli.Mail.List.Max != 50 {
			t.Fatalf("unexpected max: %d", cli.Mail.List.Max)
		}
		if cli.Mail.List.Query != "isRead:false" {
			t.Fatalf("unexpected query: %q", cli.Mail.List.Query)
		}
	})

	t.Run("folders include hidden and app-only user", func(t *testing.T) {
		parser, cli, err := newParser("test")
		if err != nil {
			t.Fatalf("newParser failed: %v", err)
		}

		args := []string{
			"mail", "folders",
			"--include-hidden",
			"--max", "25",
			"--user", "person@example.com",
		}
		if _, err := parser.Parse(args); err != nil {
			t.Fatalf("parse failed: %v", err)
		}

		if !cli.Mail.Folders.IncludeHidden {
			t.Fatal("expected --include-hidden to be set")
		}
		if cli.Mail.Folders.Max != 25 {
			t.Fatalf("unexpected max: %d", cli.Mail.Folders.Max)
		}
		if cli.Mail.Folders.User != "person@example.com" {
			t.Fatalf("unexpected user: %q", cli.Mail.Folders.User)
		}
	})
}

func TestMailReadCommandsRejectUnboundedMax(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "messages",
			run:  func() error { return (&MailListCmd{Max: 0}).Run(context.Background()) },
		},
		{
			name: "folders",
			run:  func() error { return (&MailFoldersCmd{Max: -1}).Run(context.Background()) },
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := tc.run()
			if err == nil {
				t.Fatal("expected usage error")
			}
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != 2 {
				t.Fatalf("expected usage ExitError code 2, got %v", err)
			}
			if !strings.Contains(err.Error(), "--max must be greater than zero") {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestMailPagePayloadReportsCoverageFromNextLink(t *testing.T) {
	items := []map[string]any{{"id": "1"}}
	tests := []struct {
		name         string
		itemKey      string
		next         string
		wantComplete bool
		wantHasMore  bool
	}{
		{name: "message page continues", itemKey: "messages", next: "https://graph.microsoft.com/messages/next", wantHasMore: true},
		{name: "folder page is complete", itemKey: "folders", wantComplete: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			payload := mailPagePayload(tc.itemKey, items, tc.next)
			if payload["next"] != tc.next {
				t.Fatalf("opaque next link changed: got %#v want %q", payload["next"], tc.next)
			}
			if payload["complete"] != tc.wantComplete {
				t.Fatalf("unexpected complete metadata: %#v", payload["complete"])
			}
			if payload["hasMore"] != tc.wantHasMore {
				t.Fatalf("unexpected hasMore metadata: %#v", payload["hasMore"])
			}
			if !reflect.DeepEqual(payload[tc.itemKey], items) {
				t.Fatalf("existing item field changed: %#v", payload[tc.itemKey])
			}
		})
	}
}

func TestMailSendQuoteFlagParsesWithoutBody(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	args := []string{
		"mail", "send",
		"--to", "dev@example.com",
		"--subject", "Re: Hello",
		"--quote", "message-id-123",
	}
	if _, err := parser.Parse(args); err != nil {
		t.Fatalf("parse failed: %v", err)
	}

	if cli.Mail.Send.Body != "" {
		t.Fatalf("expected empty body, got %q", cli.Mail.Send.Body)
	}
	if cli.Mail.Send.Quote != "message-id-123" {
		t.Fatalf("unexpected quote id: %q", cli.Mail.Send.Quote)
	}
}

func TestMailMutationFlagsParse(t *testing.T) {
	parser, cli, err := newParser("test")
	if err != nil {
		t.Fatalf("newParser failed: %v", err)
	}

	if _, err := parser.Parse([]string{
		"mail", "move", "message-id",
		"--folder", "archive",
		"--user", "person@example.com",
		"--dry-run",
	}); err != nil {
		t.Fatalf("parse move failed: %v", err)
	}
	if cli.Mail.Move.ID != "message-id" || cli.Mail.Move.Folder != "archive" ||
		cli.Mail.Move.User != "person@example.com" || !cli.Mail.Move.DryRun {
		t.Fatalf("unexpected move command: %#v", cli.Mail.Move)
	}
}

func TestMailMutationDryRunsNeedNoProfileAndRenderStableJSON(t *testing.T) {
	t.Setenv("MOG_PROFILE", "profile-that-must-not-be-resolved-for-dry-run")
	t.Setenv("MOG_ENABLE_COMMANDS", "")
	t.Setenv("MOG_ENABLE_ACTIONS", "")
	t.Setenv("MOG_MANAGED_AUTOMATION", "")
	t.Setenv("MOG_JSON", "")
	t.Setenv("MOG_PLAIN", "")

	tests := []struct {
		name string
		args []string
		want map[string]any
	}{
		{
			name: "archive",
			args: []string{"mail", "archive", "message/id", "--dry-run", "--json"},
			want: map[string]any{
				"action":      "mail.archive",
				"destination": "archive",
				"dry_run":     true,
				"id":          "message/id",
			},
		},
		{
			name: "move",
			args: []string{"mail", "move", "message/id", "--folder", "folder-id", "--dry-run", "--json"},
			want: map[string]any{
				"action":      "mail.move",
				"destination": "folder-id",
				"dry_run":     true,
				"id":          "message/id",
			},
		},
		{
			name: "mark read",
			args: []string{"mail", "mark-read", "message/id", "--dry-run", "--json"},
			want: map[string]any{
				"action":  "mail.mark-read",
				"dry_run": true,
				"id":      "message/id",
				"is_read": true,
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := captureExecuteOutput(t, tc.args)
			if err != nil {
				t.Fatalf("dry run failed without a usable profile: %v stderr=%q", err, stderr)
			}

			var got map[string]any
			if err := json.Unmarshal([]byte(stdout), &got); err != nil {
				t.Fatalf("decode dry-run JSON: %v\n%s", err, stdout)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("unexpected dry-run output:\n got: %#v\nwant: %#v", got, tc.want)
			}
		})
	}
}

func TestMailMutationDryRunsRenderStablePlainOutput(t *testing.T) {
	t.Setenv("MOG_PROFILE", "profile-that-must-not-be-resolved-for-dry-run")
	t.Setenv("MOG_ENABLE_COMMANDS", "")
	t.Setenv("MOG_ENABLE_ACTIONS", "")
	t.Setenv("MOG_MANAGED_AUTOMATION", "")
	t.Setenv("MOG_JSON", "")
	t.Setenv("MOG_PLAIN", "")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "archive",
			args: []string{"mail", "archive", "message-id", "--dry-run", "--plain"},
			want: "Dry run: would archive message message-id to folder archive\n",
		},
		{
			name: "move",
			args: []string{"mail", "move", "message-id", "--folder", "folder-id", "--dry-run", "--plain"},
			want: "Dry run: would move message message-id to folder folder-id\n",
		},
		{
			name: "mark read",
			args: []string{"mail", "mark-read", "message-id", "--dry-run", "--plain"},
			want: "Dry run: would mark message message-id as read (isRead=true)\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, err := captureExecuteOutput(t, tc.args)
			if err != nil {
				t.Fatalf("plain dry run failed: %v stderr=%q", err, stderr)
			}
			if stdout != tc.want {
				t.Fatalf("unexpected plain output: got %q want %q", stdout, tc.want)
			}
		})
	}
}

func TestMailMutationWhitespaceInputsFailBeforeProfileResolution(t *testing.T) {
	t.Setenv("MOG_PROFILE", "profile-that-must-not-be-resolved")
	t.Setenv("MOG_ENABLE_COMMANDS", "")
	t.Setenv("MOG_ENABLE_ACTIONS", "")
	t.Setenv("MOG_MANAGED_AUTOMATION", "")

	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "archive id", args: []string{"mail", "archive", " ", "--dry-run"}, want: "message id is required"},
		{name: "move id", args: []string{"mail", "move", " ", "--folder", "archive", "--dry-run"}, want: "message id is required"},
		{name: "move folder", args: []string{"mail", "move", "message-id", "--folder", " ", "--dry-run"}, want: "destination folder is required"},
		{name: "mark-read id", args: []string{"mail", "mark-read", " ", "--dry-run"}, want: "message id is required"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, err := captureExecuteOutput(t, tc.args)
			if err == nil || !strings.Contains(stderr, tc.want) {
				t.Fatalf("expected validation error containing %q, err=%v stderr=%q", tc.want, err, stderr)
			}
			if strings.Contains(stderr, "profile") {
				t.Fatalf("validation must occur before profile resolution: %q", stderr)
			}
		})
	}
}

func TestComposeQuotedReplyBodyAppendsQuoteBlock(t *testing.T) {
	source := map[string]any{
		"sentDateTime": "2026-02-18T13:00:00Z",
		"from": map[string]any{
			"emailAddress": map[string]any{
				"address": "sender@example.com",
			},
		},
		"bodyPreview": "Hello there\nHow are you?",
	}

	got := composeQuotedReplyBody("Thanks for the update.", source)
	wantContains := []string{
		"Thanks for the update.",
		"On 2026-02-18T13:00:00Z, sender@example.com wrote:",
		"> Hello there",
		"> How are you?",
	}
	for _, fragment := range wantContains {
		if !strings.Contains(got, fragment) {
			t.Fatalf("expected reply body to contain %q, got:\n%s", fragment, got)
		}
	}
}

func TestComposeQuotedReplyBodyAllowsQuoteOnly(t *testing.T) {
	source := map[string]any{
		"bodyPreview": "Only quoted text",
	}

	got := composeQuotedReplyBody("", source)
	if strings.Contains(got, "\n\n") {
		t.Fatalf("expected quote-only body without leading empty section, got %q", got)
	}
	if !strings.Contains(got, "sender wrote:") {
		t.Fatalf("expected quote header, got %q", got)
	}
	if !strings.Contains(got, "> Only quoted text") {
		t.Fatalf("expected quoted message content, got %q", got)
	}
}
