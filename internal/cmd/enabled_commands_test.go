package cmd

import "testing"

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

			err = enforceEnabledActions(kctx, tc.enabled)
			if tc.wantErr && err == nil {
				t.Fatal("expected action to be blocked")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected action to be allowed: %v", err)
			}
		})
	}
}
