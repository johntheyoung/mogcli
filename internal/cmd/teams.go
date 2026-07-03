package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/outfmt"
	teamsvc "github.com/jaredpalmer/mogcli/internal/services/teams"
)

type TeamsCmd struct {
	List        TeamsListCmd        `cmd:"" help:"List joined teams"`
	Channels    TeamsChannelsCmd    `cmd:"" help:"List channels in a team"`
	ChannelSend TeamsChannelSendCmd `cmd:"" name:"channel-send" help:"Send a message to a team channel"`
	Chats       TeamsChatsCmd       `cmd:"" help:"List Teams chats"`
	ChatMembers TeamsChatMembersCmd `cmd:"" name:"chat-members" help:"List members in a Teams chat"`
	ChatSend    TeamsChatSendCmd    `cmd:"" name:"chat-send" help:"Send a message to a Teams chat"`
	DMSend      TeamsDMSendCmd      `cmd:"" name:"dm-send" help:"Send a direct Teams message"`
}

type TeamsListCmd struct {
	Max  int    `name:"max" default:"100" help:"Maximum teams"`
	Page string `name:"page" aliases:"next-token" help:"Resume from next page token"`
}

func (c *TeamsListCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capTeamsList)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	items, next, err := teamsvc.New(rt.Graph).List(ctx, c.Max, page)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"teams": items, "next": next})
	}
	printItemTable(ctx, items, []string{"displayName", "id", "description"})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

type TeamsChannelsCmd struct {
	Team string `name:"team" required:"" help:"Team ID"`
	Max  int    `name:"max" default:"100" help:"Maximum channels"`
	Page string `name:"page" aliases:"next-token" help:"Resume from next page token"`
}

func (c *TeamsChannelsCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capTeamsChannels)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	items, next, err := teamsvc.New(rt.Graph).Channels(ctx, c.Team, c.Max, page)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"channels": items, "next": next})
	}
	printItemTable(ctx, items, []string{"displayName", "id", "membershipType", "isArchived", "description"})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

type TeamsChatsCmd struct {
	Max  int    `name:"max" default:"50" help:"Maximum chats"`
	Page string `name:"page" aliases:"next-token" help:"Resume from next page token"`
}

func (c *TeamsChatsCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capTeamsChats)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	items, next, err := teamsvc.New(rt.Graph).Chats(ctx, c.Max, page)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"chats": items, "next": next})
	}
	printItemTable(ctx, items, []string{"topic", "chatType", "id", "lastUpdatedDateTime", "webUrl"})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

type TeamsChatMembersCmd struct {
	Chat string `name:"chat" required:"" help:"Chat ID"`
	Max  int    `name:"max" default:"50" help:"Maximum chat members"`
	Page string `name:"page" aliases:"next-token" help:"Resume from next page token"`
}

func (c *TeamsChatMembersCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capTeamsChatMembers)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	items, next, err := teamsvc.New(rt.Graph).ChatMembers(ctx, c.Chat, c.Max, page)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"members": items, "next": next})
	}
	printItemTable(ctx, items, []string{"displayName", "email", "userId", "id", "roles"})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

type TeamsChannelSendCmd struct {
	Team        string `name:"team" required:"" help:"Team ID"`
	Channel     string `name:"channel" required:"" help:"Channel ID"`
	Body        string `name:"body" required:"" help:"Message body"`
	ContentType string `name:"content-type" enum:"text,html" default:"text" help:"Message body content type"`
	DryRun      bool   `name:"dry-run" help:"Preview send without posting the message"`
}

func (c *TeamsChannelSendCmd) Run(ctx context.Context) error {
	body := strings.TrimSpace(c.Body)
	if body == "" {
		return usage("--body is required")
	}

	rt, err := resolveRuntime(ctx, capTeamsSend)
	if err != nil {
		return err
	}

	if c.DryRun {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{
				"dry_run":     true,
				"action":      "teams.channel-send",
				"team":        c.Team,
				"channel":     c.Channel,
				"contentType": c.ContentType,
				"body_len":    len(body),
			})
		}
		fmt.Fprintf(os.Stdout, "Dry run: would send Teams channel message to team %s channel %s\n", c.Team, c.Channel)
		return nil
	}

	item, err := teamsvc.New(rt.Graph).SendChannelMessage(ctx, c.Team, c.Channel, body, c.ContentType)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, item)
	}
	fmt.Fprintf(os.Stdout, "Sent Teams channel message %s\n", flattenValue(item["id"]))
	return nil
}

type TeamsChatSendCmd struct {
	Chat        string   `name:"chat" required:"" help:"Chat ID"`
	Body        string   `name:"body" required:"" help:"Message body"`
	ContentType string   `name:"content-type" enum:"text,html" default:"text" help:"Message body content type"`
	Mention     []string `name:"mention" help:"Teams @mention as Display Name:<aad-object-id> (repeat for multiple)"`
	DryRun      bool     `name:"dry-run" help:"Preview send without posting the message"`
}

func (c *TeamsChatSendCmd) Run(ctx context.Context) error {
	body := strings.TrimSpace(c.Body)
	if body == "" {
		return usage("--body is required")
	}
	mentions, err := teamsvc.ParseChatMentions(c.Mention)
	if err != nil {
		return usage(err.Error())
	}

	rt, err := resolveRuntime(ctx, capTeamsChatSend)
	if err != nil {
		return err
	}

	if c.DryRun {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, teamsChatSendDryRunPayload(c.Chat, body, c.ContentType, mentions))
		}
		if len(mentions) > 0 {
			fmt.Fprintf(os.Stdout, "Dry run: would send Teams chat message to chat %s with %d mention(s): %s\n", c.Chat, len(mentions), strings.Join(teamsvc.ChatMentionDisplayNames(mentions), ", "))
			return nil
		}
		fmt.Fprintf(os.Stdout, "Dry run: would send Teams chat message to chat %s\n", c.Chat)
		return nil
	}

	item, err := teamsvc.New(rt.Graph).SendChatMessageWithMentions(ctx, c.Chat, body, c.ContentType, mentions)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, item)
	}
	fmt.Fprintf(os.Stdout, "Sent Teams chat message %s\n", flattenValue(item["id"]))
	return nil
}

func teamsChatSendDryRunPayload(chat string, body string, contentType string, mentions []teamsvc.ChatMention) map[string]any {
	payload := map[string]any{
		"dry_run":     true,
		"action":      "teams.chat-send",
		"chat":        chat,
		"contentType": contentType,
		"body_len":    len(body),
	}
	if len(mentions) == 0 {
		return payload
	}

	payload["contentType"] = "html"
	payload["mention_count"] = len(mentions)
	payload["mention_display_names"] = teamsvc.ChatMentionDisplayNames(mentions)
	return payload
}

type TeamsDMSendCmd struct {
	To          string `name:"to" required:"" help:"Target user ID or user principal name"`
	Body        string `name:"body" required:"" help:"Message body"`
	ContentType string `name:"content-type" enum:"text,html" default:"text" help:"Message body content type"`
	DryRun      bool   `name:"dry-run" help:"Preview send without opening a chat or posting the message"`
}

func (c *TeamsDMSendCmd) Run(ctx context.Context) error {
	to := strings.TrimSpace(c.To)
	if to == "" {
		return usage("--to is required")
	}
	body := strings.TrimSpace(c.Body)
	if body == "" {
		return usage("--body is required")
	}

	rt, err := resolveRuntime(ctx, capTeamsDMSend)
	if err != nil {
		return err
	}

	if c.DryRun {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{
				"dry_run":     true,
				"action":      "teams.dm-send",
				"to":          to,
				"contentType": c.ContentType,
				"body_len":    len(body),
			})
		}
		fmt.Fprintf(os.Stdout, "Dry run: would send Teams DM to %s\n", to)
		return nil
	}

	svc := teamsvc.New(rt.Graph)
	chat, err := svc.OpenOneOnOneChat(ctx, to)
	if err != nil {
		return err
	}
	chatID := flattenValue(chat["id"])
	if chatID == "" {
		return fmt.Errorf("create one-on-one chat returned no id")
	}

	item, err := svc.SendChatMessage(ctx, chatID, body, c.ContentType)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{
			"chat":    chat,
			"message": item,
		})
	}
	fmt.Fprintf(os.Stdout, "Sent Teams DM %s in chat %s\n", flattenValue(item["id"]), chatID)
	return nil
}
