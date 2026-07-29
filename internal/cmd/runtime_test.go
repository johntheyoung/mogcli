package cmd

import (
	"errors"
	"strings"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/config"
	"github.com/jaredpalmer/mogcli/internal/errfmt"
	"github.com/jaredpalmer/mogcli/internal/profile"
)

func TestValidateCapability(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name         string
		record       config.ProfileRecord
		capability   runtimeCapability
		wantError    bool
		wantContains string
	}{
		{
			name: "consumer delegated mail list is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceConsumer,
				AuthMode: profile.AuthModeDelegated,
			},
			capability: capMailList,
		},
		{
			name: "enterprise app-only mail list is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability: capMailList,
		},
		{
			name: "enterprise app-only mail folders is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability: capMailFolders,
		},
		{
			name: "consumer delegated mail archive is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceConsumer,
				AuthMode: profile.AuthModeDelegated,
			},
			capability: capMailArchive,
		},
		{
			name: "enterprise app-only mail move is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability: capMailMove,
		},
		{
			name: "consumer delegated mail mark-read is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceConsumer,
				AuthMode: profile.AuthModeDelegated,
			},
			capability: capMailMarkRead,
		},
		{
			name: "enterprise app-only groups list is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability: capGroupsList,
		},
		{
			name: "consumer delegated groups list is blocked",
			record: config.ProfileRecord{
				Audience: profile.AudienceConsumer,
				AuthMode: profile.AuthModeDelegated,
			},
			capability:   capGroupsList,
			wantError:    true,
			wantContains: "requires an enterprise profile",
		},
		{
			name: "enterprise app-only calendar list is blocked",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability:   capCalendarList,
			wantError:    true,
			wantContains: "calendar commands are not supported in app-only mode",
		},
		{
			name: "enterprise app-only tasks delete is blocked",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability:   capTasksDelete,
			wantError:    true,
			wantContains: "tasks commands are not supported in app-only mode",
		},
		{
			name: "enterprise delegated tasks delete is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeDelegated,
			},
			capability: capTasksDelete,
		},
		{
			name: "enterprise delegated Teams chat file send is allowed",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeDelegated,
			},
			capability: capTeamsChatFileSend,
		},
		{
			name: "consumer delegated Teams chat file send is blocked",
			record: config.ProfileRecord{
				Audience: profile.AudienceConsumer,
				AuthMode: profile.AuthModeDelegated,
			},
			capability:   capTeamsChatFileSend,
			wantError:    true,
			wantContains: "enterprise profile",
		},
		{
			name: "enterprise app-only Teams chat file send is blocked",
			record: config.ProfileRecord{
				Audience: profile.AudienceEnterprise,
				AuthMode: profile.AuthModeAppOnly,
			},
			capability:   capTeamsChatFileSend,
			wantError:    true,
			wantContains: "not supported in app-only mode",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			err := validateCapability(tc.record, tc.capability)
			if tc.wantError {
				if err == nil {
					t.Fatal("expected error but got nil")
				}
				var userErr *errfmt.UserFacingError
				if !errors.As(err, &userErr) {
					t.Fatalf("expected UserFacingError, got %T", err)
				}
				if tc.wantContains != "" && !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.wantContains)) {
					t.Fatalf("expected error to contain %q, got %q", tc.wantContains, err.Error())
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
		})
	}
}

func TestResolveAppOnlyTargetUser(t *testing.T) {
	t.Parallel()

	t.Run("delegated rejects user override", func(t *testing.T) {
		t.Parallel()

		_, err := resolveAppOnlyTargetUser(config.ProfileRecord{
			AuthMode: profile.AuthModeDelegated,
		}, "person@example.com")
		if err == nil {
			t.Fatal("expected error")
		}

		var exitErr *ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != 2 {
			t.Fatalf("expected usage ExitError code 2, got %v", err)
		}
	})

	t.Run("delegated without override returns empty target", func(t *testing.T) {
		t.Parallel()

		target, err := resolveAppOnlyTargetUser(config.ProfileRecord{
			AuthMode: profile.AuthModeDelegated,
		}, "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if target != "" {
			t.Fatalf("expected empty target, got %q", target)
		}
	})

	t.Run("app-only uses override before profile default", func(t *testing.T) {
		t.Parallel()

		target, err := resolveAppOnlyTargetUser(config.ProfileRecord{
			AuthMode:    profile.AuthModeAppOnly,
			AppOnlyUser: "default@example.com",
		}, "override@example.com")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if target != "override@example.com" {
			t.Fatalf("expected override target, got %q", target)
		}
	})

	t.Run("app-only uses profile default when override empty", func(t *testing.T) {
		t.Parallel()

		target, err := resolveAppOnlyTargetUser(config.ProfileRecord{
			AuthMode:    profile.AuthModeAppOnly,
			AppOnlyUser: "default@example.com",
		}, "")
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}
		if target != "default@example.com" {
			t.Fatalf("expected profile default target, got %q", target)
		}
	})

	t.Run("app-only requires target user", func(t *testing.T) {
		t.Parallel()

		_, err := resolveAppOnlyTargetUser(config.ProfileRecord{
			AuthMode: profile.AuthModeAppOnly,
		}, "")
		if err == nil {
			t.Fatal("expected error")
		}

		var userErr *errfmt.UserFacingError
		if !errors.As(err, &userErr) {
			t.Fatalf("expected UserFacingError, got %T", err)
		}
		if !strings.Contains(strings.ToLower(err.Error()), "require a target user") {
			t.Fatalf("unexpected error: %v", err)
		}
	})
}
