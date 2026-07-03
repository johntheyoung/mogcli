package teams

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listTeamsScopes = []string{"Team.ReadBasic.All"}
var listChannelsScopes = []string{"Channel.ReadBasic.All"}
var sendChannelMessageScopes = []string{"ChannelMessage.Send"}
var meScopes = []string{"User.Read"}
var listChatsScopes = []string{"Chat.ReadBasic"}
var listChatMembersScopes = []string{"ChatMember.Read"}
var createChatScopes = []string{"Chat.Create"}
var sendChatMessageScopes = []string{"ChatMessage.Send"}

type Service struct {
	client *graph.Client
}

func New(client *graph.Client) *Service {
	return &Service{client: client}
}

func (s *Service) List(ctx context.Context, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("$select", "id,displayName,description")

	endpoint := "/me/joinedTeams"
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listTeamsScopes, nil)
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	trimmed, trimmedNext := trimPage(items, next, max)
	return trimmed, trimmedNext, nil
}

func (s *Service) Channels(ctx context.Context, teamID string, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("$select", "id,displayName,description,membershipType,isArchived")

	endpoint := "/teams/" + url.PathEscape(strings.TrimSpace(teamID)) + "/channels"
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listChannelsScopes, nil)
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	trimmed, trimmedNext := trimPage(items, next, max)
	return trimmed, trimmedNext, nil
}

func (s *Service) Chats(ctx context.Context, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("$select", "id,topic,chatType,createdDateTime,lastUpdatedDateTime,webUrl")
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", chatPageSize(max)))
	}

	endpoint := "/me/chats"
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listChatsScopes, nil)
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	trimmed, trimmedNext := trimPage(items, next, max)
	return trimmed, trimmedNext, nil
}

func (s *Service) ChatMembers(ctx context.Context, chatID string, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("$select", "id,displayName,userId,roles")
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", chatPageSize(max)))
	}

	endpoint := "/chats/" + url.PathEscape(strings.TrimSpace(chatID)) + "/members"
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listChatMembersScopes, nil)
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	trimmed, trimmedNext := trimPage(items, next, max)
	return trimmed, trimmedNext, nil
}

func (s *Service) SendChannelMessage(ctx context.Context, teamID string, channelID string, body string, contentType string) (map[string]any, error) {
	payload := map[string]any{
		"body": map[string]any{
			"contentType": normalizeContentType(contentType),
			"content":     strings.TrimSpace(body),
		},
	}

	var created map[string]any
	endpoint := "/teams/" + url.PathEscape(strings.TrimSpace(teamID)) + "/channels/" + url.PathEscape(strings.TrimSpace(channelID)) + "/messages"
	err := s.client.DoJSON(ctx, http.MethodPost, endpoint, nil, payload, sendChannelMessageScopes, &created)
	return created, err
}

func (s *Service) Me(ctx context.Context) (map[string]any, error) {
	query := url.Values{}
	query.Set("$select", "id,userPrincipalName,mail")

	var item map[string]any
	err := s.client.DoJSON(ctx, http.MethodGet, "/me", query, nil, meScopes, &item)
	return item, err
}

func (s *Service) CreateOneOnOneChat(ctx context.Context, currentUser string, targetUser string) (map[string]any, error) {
	currentUser = strings.TrimSpace(currentUser)
	targetUser = strings.TrimSpace(targetUser)
	if currentUser == "" {
		return nil, fmt.Errorf("current user is required")
	}
	if targetUser == "" {
		return nil, fmt.Errorf("target user is required")
	}

	payload := map[string]any{
		"chatType": "oneOnOne",
		"members": []map[string]any{
			{
				"@odata.type":     "#microsoft.graph.aadUserConversationMember",
				"roles":           []string{"owner"},
				"user@odata.bind": graphUserBinding(currentUser),
			},
			{
				"@odata.type":     "#microsoft.graph.aadUserConversationMember",
				"roles":           []string{"owner"},
				"user@odata.bind": graphUserBinding(targetUser),
			},
		},
	}

	var created map[string]any
	err := s.client.DoJSON(ctx, http.MethodPost, "/chats", nil, payload, createChatScopes, &created)
	return created, err
}

func (s *Service) OpenOneOnOneChat(ctx context.Context, targetUser string) (map[string]any, error) {
	me, err := s.Me(ctx)
	if err != nil {
		return nil, err
	}

	currentUser := stringField(me, "id")
	if currentUser == "" {
		currentUser = stringField(me, "userPrincipalName")
	}
	if currentUser == "" {
		return nil, fmt.Errorf("signed-in user response did not include id or userPrincipalName")
	}

	return s.CreateOneOnOneChat(ctx, currentUser, targetUser)
}

func (s *Service) SendChatMessage(ctx context.Context, chatID string, body string, contentType string) (map[string]any, error) {
	return s.SendChatMessageWithMentions(ctx, chatID, body, contentType, nil)
}

func (s *Service) SendChatMessageWithMentions(ctx context.Context, chatID string, body string, contentType string, mentions []ChatMention) (map[string]any, error) {
	payload, err := buildChatMentionsPayload(body, contentType, mentions)
	if err != nil {
		return nil, err
	}

	var created map[string]any
	endpoint := "/chats/" + url.PathEscape(strings.TrimSpace(chatID)) + "/messages"
	err = s.client.DoJSON(ctx, http.MethodPost, endpoint, nil, payload, sendChatMessageScopes, &created)
	return created, err
}

func normalizeContentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "html":
		return "html"
	default:
		return "text"
	}
}

func chatPageSize(max int) int {
	if max <= 0 {
		return 0
	}
	if max > 50 {
		return 50
	}
	return max
}

func graphUserBinding(identifier string) string {
	escaped := strings.ReplaceAll(strings.TrimSpace(identifier), "'", "''")
	return "https://graph.microsoft.com/v1.0/users('" + escaped + "')"
}

func stringField(item map[string]any, key string) string {
	if item == nil {
		return ""
	}
	value, ok := item[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func trimPage(items []map[string]any, next string, max int) ([]map[string]any, string) {
	if max <= 0 || len(items) <= max {
		return items, next
	}
	return items[:max], next
}
