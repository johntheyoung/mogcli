package teams

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listTeamsScopes = []string{"Team.ReadBasic.All"}
var listChannelsScopes = []string{"Channel.ReadBasic.All"}
var sendChannelMessageScopes = []string{"ChannelMessage.Send"}

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

func normalizeContentType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "html":
		return "html"
	default:
		return "text"
	}
}

func trimPage(items []map[string]any, next string, max int) ([]map[string]any, string) {
	if max <= 0 || len(items) <= max {
		return items, next
	}
	return items[:max], next
}
