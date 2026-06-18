package cmd

import (
	"context"
	"fmt"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/auth"
	"github.com/jaredpalmer/mogcli/internal/config"
	"github.com/jaredpalmer/mogcli/internal/errfmt"
	"github.com/jaredpalmer/mogcli/internal/graph"
	"github.com/jaredpalmer/mogcli/internal/profile"
)

type runtimeEnv struct {
	Profile config.ProfileRecord
	Auth    *auth.Manager
	Graph   *graph.Client
}

type runtimeCapability string

const (
	capMailList       runtimeCapability = "mail.list"
	capMailGet        runtimeCapability = "mail.get"
	capMailSend       runtimeCapability = "mail.send"
	capCalendarList   runtimeCapability = "calendar.list"
	capCalendarGet    runtimeCapability = "calendar.get"
	capCalendarCreate runtimeCapability = "calendar.create"
	capCalendarUpdate runtimeCapability = "calendar.update"
	capCalendarDelete runtimeCapability = "calendar.delete"
	capContactsList   runtimeCapability = "contacts.list"
	capContactsGet    runtimeCapability = "contacts.get"
	capContactsCreate runtimeCapability = "contacts.create"
	capContactsUpdate runtimeCapability = "contacts.update"
	capContactsDelete runtimeCapability = "contacts.delete"
	capGroupsList     runtimeCapability = "groups.list"
	capGroupsGet      runtimeCapability = "groups.get"
	capGroupsMembers  runtimeCapability = "groups.members"
	capTeamsList      runtimeCapability = "teams.list"
	capTeamsChannels  runtimeCapability = "teams.channels"
	capTeamsSend      runtimeCapability = "teams.channel-send"
	capTasksLists     runtimeCapability = "tasks.lists"
	capTasksList      runtimeCapability = "tasks.list"
	capTasksGet       runtimeCapability = "tasks.get"
	capTasksCreate    runtimeCapability = "tasks.create"
	capTasksUpdate    runtimeCapability = "tasks.update"
	capTasksComplete  runtimeCapability = "tasks.complete"
	capTasksDelete    runtimeCapability = "tasks.delete"
	capOneDriveLS     runtimeCapability = "onedrive.ls"
	capOneDriveGet    runtimeCapability = "onedrive.get"
	capOneDrivePut    runtimeCapability = "onedrive.put"
	capOneDriveMkdir  runtimeCapability = "onedrive.mkdir"
	capOneDriveRM     runtimeCapability = "onedrive.rm"
)

const (
	enterpriseOnlyProfileMessage = "This command requires an enterprise profile. Use `mog auth use <enterprise-profile>`."
	appOnlyEnterpriseOnlyMessage = "App-only mode is enterprise-only."
	calendarAppOnlyMessage       = "Calendar commands are not supported in app-only mode. Use a delegated profile (`mog auth use <profile>`) or re-login without `--mode app-only`."
	tasksAppOnlyMessage          = "Tasks commands are not supported in app-only mode. Use a delegated profile (`mog auth use <profile>`) or re-login without `--mode app-only`."
)

type capabilityRule struct {
	allowedAudiences           []string
	allowedAuthModes           []string
	unsupportedAudienceMessage string
	unsupportedAuthModeMessage string
}

var capabilityRules = map[runtimeCapability]capabilityRule{
	capMailList:       delegatedOrAppOnlyRule(),
	capMailGet:        delegatedOrAppOnlyRule(),
	capMailSend:       delegatedOrAppOnlyRule(),
	capCalendarList:   delegatedOnlyRule(calendarAppOnlyMessage),
	capCalendarGet:    delegatedOnlyRule(calendarAppOnlyMessage),
	capCalendarCreate: delegatedOnlyRule(calendarAppOnlyMessage),
	capCalendarUpdate: delegatedOnlyRule(calendarAppOnlyMessage),
	capCalendarDelete: delegatedOnlyRule(calendarAppOnlyMessage),
	capContactsList:   delegatedOrAppOnlyRule(),
	capContactsGet:    delegatedOrAppOnlyRule(),
	capContactsCreate: delegatedOrAppOnlyRule(),
	capContactsUpdate: delegatedOrAppOnlyRule(),
	capContactsDelete: delegatedOrAppOnlyRule(),
	capGroupsList:     groupsRule(),
	capGroupsGet:      groupsRule(),
	capGroupsMembers:  groupsRule(),
	capTeamsList:      teamsRule(),
	capTeamsChannels:  teamsRule(),
	capTeamsSend:      teamsRule(),
	capTasksLists:     delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksList:      delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksGet:       delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksCreate:    delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksUpdate:    delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksComplete:  delegatedOnlyRule(tasksAppOnlyMessage),
	capTasksDelete:    delegatedOnlyRule(tasksAppOnlyMessage),
	capOneDriveLS:     delegatedOrAppOnlyRule(),
	capOneDriveGet:    delegatedOrAppOnlyRule(),
	capOneDrivePut:    delegatedOrAppOnlyRule(),
	capOneDriveMkdir:  delegatedOrAppOnlyRule(),
	capOneDriveRM:     delegatedOrAppOnlyRule(),
}

func delegatedOnlyRule(message string) capabilityRule {
	return capabilityRule{
		allowedAudiences: []string{
			profile.AudienceConsumer,
			profile.AudienceEnterprise,
		},
		allowedAuthModes: []string{
			profile.AuthModeDelegated,
		},
		unsupportedAuthModeMessage: message,
	}
}

func delegatedOrAppOnlyRule() capabilityRule {
	return capabilityRule{
		allowedAudiences: []string{
			profile.AudienceConsumer,
			profile.AudienceEnterprise,
		},
		allowedAuthModes: []string{
			profile.AuthModeDelegated,
			profile.AuthModeAppOnly,
		},
	}
}

func groupsRule() capabilityRule {
	return capabilityRule{
		allowedAudiences: []string{
			profile.AudienceEnterprise,
		},
		allowedAuthModes: []string{
			profile.AuthModeDelegated,
			profile.AuthModeAppOnly,
		},
		unsupportedAudienceMessage: enterpriseOnlyProfileMessage,
	}
}

func teamsRule() capabilityRule {
	return capabilityRule{
		allowedAudiences: []string{
			profile.AudienceEnterprise,
		},
		allowedAuthModes: []string{
			profile.AuthModeDelegated,
		},
		unsupportedAudienceMessage: "Teams commands require an enterprise profile. Use `mog auth use <enterprise-profile>`.",
		unsupportedAuthModeMessage: "Teams commands are not supported in app-only mode. Use a delegated profile (`mog auth use <profile>`) or re-login without `--mode app-only`.",
	}
}

func resolveRuntime(ctx context.Context, capability runtimeCapability) (*runtimeEnv, error) {
	flags := rootFlagsFromContext(ctx)
	profileOverride := ""
	if flags != nil {
		profileOverride = flags.Profile
	}

	store := profile.NewStore()
	activeProfile, err := store.Resolve(profileOverride)
	if err != nil {
		return nil, fmt.Errorf("resolve active profile: %w", err)
	}

	if err := validateCapability(activeProfile, capability); err != nil {
		return nil, err
	}

	authManager := auth.NewManager()
	graphClient := graph.NewClient(func(ctx context.Context, scopes []string) (string, error) {
		switch normalizeAuthMode(activeProfile.AuthMode) {
		case profile.AuthModeAppOnly:
			if !strings.EqualFold(activeProfile.Audience, profile.AudienceEnterprise) {
				return "", errfmt.NewUserFacingError(appOnlyEnterpriseOnlyMessage, nil)
			}
			return authManager.AcquireAppOnlyToken(ctx, activeProfile.Name, activeProfile.ClientID, activeProfile.Authority)
		default:
			return authManager.AcquireDelegatedToken(
				ctx,
				activeProfile.Name,
				activeProfile.ClientID,
				activeProfile.Authority,
				scopes,
				activeProfile.AccountID,
				activeProfile.TenantID,
			)
		}
	})

	return &runtimeEnv{
		Profile: activeProfile,
		Auth:    authManager,
		Graph:   graphClient,
	}, nil
}

func validateCapability(activeProfile config.ProfileRecord, capability runtimeCapability) error {
	rule, ok := capabilityRules[capability]
	if !ok {
		return fmt.Errorf("unknown runtime capability: %s", capability)
	}

	audience := normalizeAudience(activeProfile.Audience)
	authMode := normalizeAuthMode(activeProfile.AuthMode)

	if authMode == profile.AuthModeAppOnly && audience != profile.AudienceEnterprise {
		return errfmt.NewUserFacingError(appOnlyEnterpriseOnlyMessage, nil)
	}

	if !containsRuleValue(rule.allowedAudiences, audience) {
		message := strings.TrimSpace(rule.unsupportedAudienceMessage)
		if message == "" {
			message = "This command is not supported for the active profile audience."
		}

		return errfmt.NewUserFacingError(message, nil)
	}

	if !containsRuleValue(rule.allowedAuthModes, authMode) {
		message := strings.TrimSpace(rule.unsupportedAuthModeMessage)
		if message == "" {
			message = "This command is not supported for the active profile auth mode."
		}

		return errfmt.NewUserFacingError(message, nil)
	}

	return nil
}

func normalizeAudience(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeAuthMode(value string) string {
	normalized := strings.ToLower(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, "-", "_")
	if normalized == "" {
		return profile.AuthModeDelegated
	}

	return normalized
}

func containsRuleValue(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func resolveAppOnlyTargetUser(activeProfile config.ProfileRecord, override string) (string, error) {
	override = strings.TrimSpace(override)
	authMode := normalizeAuthMode(activeProfile.AuthMode)
	if override != "" && authMode != profile.AuthModeAppOnly {
		return "", usage("--user is only supported for app-only profiles")
	}

	if authMode != profile.AuthModeAppOnly {
		return "", nil
	}

	target := override
	if target == "" {
		target = strings.TrimSpace(activeProfile.AppOnlyUser)
	}
	if target == "" {
		return "", errfmt.NewUserFacingError(
			"App-only commands require a target user. Set `--user` or configure a profile default with `mog auth login --app-only-user`.",
			nil,
		)
	}

	return target, nil
}

func defaultAuthority(audience string, tenant string, explicit string) string {
	if strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}

	switch strings.ToLower(strings.TrimSpace(audience)) {
	case profile.AudienceConsumer:
		return "consumers"
	default:
		if strings.TrimSpace(tenant) != "" {
			return strings.TrimSpace(tenant)
		}
		return "organizations"
	}
}
