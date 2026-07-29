package teams

import (
	"fmt"
	"html"
	"strings"
)

type ChatMention struct {
	DisplayName string
	UserID      string
}

func ParseChatMention(value string) (ChatMention, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return ChatMention{}, fmt.Errorf("invalid --mention: expected Display Name:<aad-object-id>")
	}

	sep := strings.LastIndex(raw, ":")
	if sep < 0 {
		return ChatMention{}, fmt.Errorf("invalid --mention: expected Display Name:<aad-object-id>")
	}

	mention := ChatMention{
		DisplayName: strings.TrimSpace(raw[:sep]),
		UserID:      strings.TrimSpace(raw[sep+1:]),
	}
	if mention.DisplayName == "" {
		return ChatMention{}, fmt.Errorf("invalid --mention: display name is required")
	}
	if mention.UserID == "" {
		return ChatMention{}, fmt.Errorf("invalid --mention: aad object id is required")
	}

	return mention, nil
}

func ParseChatMentions(values []string) ([]ChatMention, error) {
	if len(values) == 0 {
		return nil, nil
	}

	mentions := make([]ChatMention, 0, len(values))
	for _, value := range values {
		mention, err := ParseChatMention(value)
		if err != nil {
			return nil, err
		}
		mentions = append(mentions, mention)
	}

	return mentions, nil
}

func ChatMentionDisplayNames(mentions []ChatMention) []string {
	names := make([]string, 0, len(mentions))
	for _, mention := range mentions {
		name := strings.TrimSpace(mention.DisplayName)
		if name == "" {
			continue
		}
		names = append(names, name)
	}

	return names
}

func buildChatMentionsPayload(body string, contentType string, mentions []ChatMention) (map[string]any, error) {
	normalizedMentions, err := normalizeChatMentions(mentions)
	if err != nil {
		return nil, err
	}

	content := strings.TrimSpace(body)
	if len(normalizedMentions) == 0 {
		return map[string]any{
			"body": map[string]any{
				"contentType": normalizeContentType(contentType),
				"content":     content,
			},
		}, nil
	}

	content = buildMentionedChatContent(content, normalizeContentType(contentType), normalizedMentions)
	payload := map[string]any{
		"body": map[string]any{
			"contentType": "html",
			"content":     content,
		},
		"mentions": buildGraphChatMentions(normalizedMentions),
	}

	return payload, nil
}

func normalizeChatMentions(mentions []ChatMention) ([]ChatMention, error) {
	if len(mentions) == 0 {
		return nil, nil
	}

	out := make([]ChatMention, 0, len(mentions))
	for i, mention := range mentions {
		item := ChatMention{
			DisplayName: strings.TrimSpace(mention.DisplayName),
			UserID:      strings.TrimSpace(mention.UserID),
		}
		if item.DisplayName == "" {
			return nil, fmt.Errorf("mention %d display name is required", i+1)
		}
		if item.UserID == "" {
			return nil, fmt.Errorf("mention %d aad object id is required", i+1)
		}
		out = append(out, item)
	}

	return out, nil
}

func buildMentionedChatContent(content string, contentType string, mentions []ChatMention) string {
	if contentType != "html" && !containsAnyChatMentionTag(content, mentions) {
		content = textToChatHTML(content)
	}

	missingTags := make([]string, 0, len(mentions))
	for i, mention := range mentions {
		tag := chatMentionTag(i, mention.DisplayName)
		if strings.Contains(content, tag) {
			continue
		}
		missingTags = append(missingTags, tag)
	}
	if len(missingTags) == 0 {
		return content
	}

	mentionLine := strings.Join(missingTags, " ")
	if strings.TrimSpace(content) == "" {
		return mentionLine
	}

	return content + "<br>" + mentionLine
}

func containsAnyChatMentionTag(content string, mentions []ChatMention) bool {
	for i, mention := range mentions {
		if strings.Contains(content, chatMentionTag(i, mention.DisplayName)) {
			return true
		}
	}

	return false
}

func textToChatHTML(content string) string {
	escaped := html.EscapeString(content)
	escaped = strings.ReplaceAll(escaped, "\r\n", "\n")
	escaped = strings.ReplaceAll(escaped, "\r", "\n")
	return strings.ReplaceAll(escaped, "\n", "<br>")
}

func chatMentionTag(id int, displayName string) string {
	return fmt.Sprintf(`<at id="%d">%s</at>`, id, html.EscapeString(strings.TrimSpace(displayName)))
}

func buildGraphChatMentions(mentions []ChatMention) []map[string]any {
	out := make([]map[string]any, 0, len(mentions))
	for i, mention := range mentions {
		out = append(out, map[string]any{
			"id":          i,
			"mentionText": mention.DisplayName,
			"mentioned": map[string]any{
				"user": map[string]any{
					"displayName":      mention.DisplayName,
					"id":               mention.UserID,
					"userIdentityType": "aadUser",
				},
			},
		})
	}

	return out
}
