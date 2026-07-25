package cmd

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/outfmt"
	"github.com/jaredpalmer/mogcli/internal/services/mail"
)

type MailCmd struct {
	List    MailListCmd    `cmd:"" help:"List one page of messages; JSON reports complete/hasMore coverage"`
	Folders MailFoldersCmd `cmd:"" help:"List one page of top-level folders; JSON reports complete/hasMore coverage"`
	Get     MailGetCmd     `cmd:"" help:"Get message by ID"`
	Send    MailSendCmd    `cmd:"" help:"Send a new message"`
}

type MailListCmd struct {
	Max    int    `name:"max" default:"20" help:"Maximum messages requested for the initial page (ignored with --page)"`
	Query  string `name:"query" help:"Search query text for the initial request"`
	Folder string `name:"folder" help:"Initial folder ID or well-known name (for example inbox); omit for mailbox-wide messages"`
	Page   string `name:"page" aliases:"next-token" help:"Resume from an opaque Graph next-page URL; initial request selectors are ignored"`
	User   string `name:"user" help:"App-only target user override (UPN or user ID)"`
}

func (c *MailListCmd) Run(ctx context.Context) error {
	if c.Max <= 0 {
		return usage("--max must be greater than zero")
	}
	rt, err := resolveRuntime(ctx, capMailList)
	if err != nil {
		return err
	}
	targetUser, err := resolveAppOnlyTargetUser(rt.Profile, c.User)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	svc := mail.New(rt.Graph, targetUser)
	items, next, err := svc.List(ctx, c.Max, c.Query, page, c.Folder)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, mailPagePayload("messages", items, next))
	}

	printItemTable(ctx, items, []string{"receivedDateTime", "subject", "id", "isRead"})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

type MailFoldersCmd struct {
	Max           int    `name:"max" default:"20" help:"Maximum folders requested for the initial page (ignored with --page)"`
	Page          string `name:"page" aliases:"next-token" help:"Resume from an opaque Graph next-page URL; initial request selectors are ignored"`
	IncludeHidden bool   `name:"include-hidden" help:"Include hidden folders in the initial request (excluded by default)"`
	User          string `name:"user" help:"App-only target user override (UPN or user ID)"`
}

func (c *MailFoldersCmd) Run(ctx context.Context) error {
	if c.Max <= 0 {
		return usage("--max must be greater than zero")
	}
	rt, err := resolveRuntime(ctx, capMailFolders)
	if err != nil {
		return err
	}
	targetUser, err := resolveAppOnlyTargetUser(rt.Profile, c.User)
	if err != nil {
		return err
	}
	page, err := normalizePageToken(c.Page)
	if err != nil {
		return err
	}

	svc := mail.New(rt.Graph, targetUser)
	items, next, err := svc.ListFolders(ctx, c.Max, page, c.IncludeHidden)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, mailPagePayload("folders", items, next))
	}

	printItemTable(ctx, items, []string{
		"id",
		"displayName",
		"parentFolderId",
		"childFolderCount",
		"totalItemCount",
		"unreadItemCount",
	})
	printNextPageHint(uiFromContext(ctx), next)
	return nil
}

func mailPagePayload(itemKey string, items []map[string]any, next string) map[string]any {
	hasMore := strings.TrimSpace(next) != ""
	return map[string]any{
		itemKey:    items,
		"next":     next,
		"hasMore":  hasMore,
		"complete": !hasMore,
	}
}

type MailGetCmd struct {
	ID   string `arg:"" required:"" help:"Message ID"`
	User string `name:"user" help:"App-only target user override (UPN or user ID)"`
}

func (c *MailGetCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capMailGet)
	if err != nil {
		return err
	}
	targetUser, err := resolveAppOnlyTargetUser(rt.Profile, c.User)
	if err != nil {
		return err
	}

	svc := mail.New(rt.Graph, targetUser)
	item, err := svc.Get(ctx, c.ID)
	if err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, item)
	}

	printSingleMap(ctx, item)
	return nil
}

type MailSendCmd struct {
	To      []string `name:"to" required:"" help:"Recipient email (repeat or comma-separate)"`
	Subject string   `name:"subject" required:"" help:"Email subject"`
	Body    string   `name:"body" help:"Plain text body"`
	Quote   string   `name:"quote" help:"Message ID to quote in reply body"`
	User    string   `name:"user" help:"App-only target user override (UPN or user ID)"`
	DryRun  bool     `name:"dry-run" help:"Preview send without sending the message"`
}

func (c *MailSendCmd) Run(ctx context.Context) error {
	rt, err := resolveRuntime(ctx, capMailSend)
	if err != nil {
		return err
	}
	targetUser, err := resolveAppOnlyTargetUser(rt.Profile, c.User)
	if err != nil {
		return err
	}

	recipients := splitCSV(c.To)
	if len(recipients) == 0 {
		return usage("at least one --to recipient is required")
	}
	if strings.TrimSpace(c.Body) == "" && strings.TrimSpace(c.Quote) == "" {
		return usage("--body is required unless --quote is provided")
	}
	if c.DryRun {
		if outfmt.IsJSON(ctx) {
			return outfmt.WriteJSON(os.Stdout, map[string]any{
				"dry_run":  true,
				"action":   "mail.send",
				"to":       recipients,
				"subject":  c.Subject,
				"body_len": len(c.Body),
				"quote":    strings.TrimSpace(c.Quote),
			})
		}
		if strings.TrimSpace(c.Quote) != "" {
			fmt.Fprintf(os.Stdout, "Dry run: would send quoted reply to %s with subject %q (quote id: %s)\n", strings.Join(recipients, ", "), c.Subject, strings.TrimSpace(c.Quote))
			return nil
		}
		fmt.Fprintf(os.Stdout, "Dry run: would send message to %s with subject %q\n", strings.Join(recipients, ", "), c.Subject)
		return nil
	}

	svc := mail.New(rt.Graph, targetUser)
	body := strings.TrimSpace(c.Body)
	quoteID := strings.TrimSpace(c.Quote)
	if quoteID != "" {
		source, getErr := svc.Get(ctx, quoteID)
		if getErr != nil {
			return getErr
		}
		body = composeQuotedReplyBody(body, source)
	}
	if err := svc.Send(ctx, recipients, c.Subject, body); err != nil {
		return err
	}

	if outfmt.IsJSON(ctx) {
		return outfmt.WriteJSON(os.Stdout, map[string]any{"sent": true, "to": recipients})
	}

	fmt.Fprintf(os.Stdout, "Sent message to %s\n", strings.Join(recipients, ", "))
	return nil
}

func splitCSV(values []string) []string {
	out := make([]string, 0)
	seen := map[string]struct{}{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			item := strings.TrimSpace(part)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}

	return out
}

func composeQuotedReplyBody(body string, source map[string]any) string {
	quoteBlock := buildQuoteBlock(source)
	body = strings.TrimSpace(body)
	if body == "" {
		return quoteBlock
	}
	return body + "\n\n" + quoteBlock
}

func buildQuoteBlock(source map[string]any) string {
	sender := messageSender(source)
	sent := strings.TrimSpace(stringValue(source["sentDateTime"]))
	content := messageQuoteContent(source)

	header := "Quoted message:"
	switch {
	case sent != "" && sender != "":
		header = fmt.Sprintf("On %s, %s wrote:", sent, sender)
	case sender != "":
		header = fmt.Sprintf("%s wrote:", sender)
	case sent != "":
		header = fmt.Sprintf("On %s:", sent)
	}

	return header + "\n" + quoteLines(content)
}

func messageSender(source map[string]any) string {
	from, ok := source["from"].(map[string]any)
	if !ok {
		return "sender"
	}

	emailAddress, ok := from["emailAddress"].(map[string]any)
	if !ok {
		return "sender"
	}

	if address := strings.TrimSpace(stringValue(emailAddress["address"])); address != "" {
		return address
	}
	if name := strings.TrimSpace(stringValue(emailAddress["name"])); name != "" {
		return name
	}
	return "sender"
}

func messageQuoteContent(source map[string]any) string {
	if preview := strings.TrimSpace(stringValue(source["bodyPreview"])); preview != "" {
		return preview
	}

	body, ok := source["body"].(map[string]any)
	if !ok {
		return "(no message body)"
	}

	content := strings.TrimSpace(stringValue(body["content"]))
	if content == "" {
		return "(no message body)"
	}

	return content
}

func quoteLines(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 {
		return "> (no message body)"
	}

	quoted := make([]string, 0, len(lines))
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			quoted = append(quoted, ">")
			continue
		}
		quoted = append(quoted, "> "+line)
	}
	return strings.Join(quoted, "\n")
}

func stringValue(v any) string {
	switch typed := v.(type) {
	case string:
		return typed
	default:
		return ""
	}
}
