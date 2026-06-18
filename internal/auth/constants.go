package auth

var BaseDelegatedScopes = []string{
	"openid",
	"profile",
	"offline_access",
	"User.Read",
}

var delegatedScopeWorkloadOrder = []string{
	"mail",
	"calendar",
	"contacts",
	"tasks",
	"onedrive",
	"groups",
	"teams",
}

var delegatedScopeWorkloadMap = map[string][]string{
	"mail": {
		"Mail.Read",
		"Mail.Send",
	},
	"calendar": {
		"Calendars.Read",
		"Calendars.ReadWrite",
	},
	"contacts": {
		"Contacts.Read",
		"Contacts.ReadWrite",
	},
	"tasks": {
		"Tasks.Read",
		"Tasks.ReadWrite",
	},
	"onedrive": {
		"Files.Read",
		"Files.ReadWrite",
	},
	"groups": {
		"Group.Read.All",
		"GroupMember.Read.All",
	},
	"teams": {
		"Team.ReadBasic.All",
		"Channel.ReadBasic.All",
		"ChannelMessage.Send",
	},
}

var AllDelegatedWorkloadScopes = []string{
	"Mail.Read",
	"Mail.Send",
	"Calendars.Read",
	"Calendars.ReadWrite",
	"Contacts.Read",
	"Contacts.ReadWrite",
	"Tasks.Read",
	"Tasks.ReadWrite",
	"Files.Read",
	"Files.ReadWrite",
	"Group.Read.All",
	"GroupMember.Read.All",
	"Team.ReadBasic.All",
	"Channel.ReadBasic.All",
	"ChannelMessage.Send",
}
