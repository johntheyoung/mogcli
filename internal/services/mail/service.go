package mail

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listMailScopes = []string{"Mail.Read"}
var getMailScopes = []string{"Mail.Read"}
var listMailFolderScopes = []string{"Mail.Read"}
var sendMailScopes = []string{"Mail.Send"}

const messageListSelect = "id,parentFolderId,conversationId,subject,from,sender,toRecipients,ccRecipients,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,importance,flag,categories,inferenceClassification,webLink"
const mailFolderListSelect = "id,displayName,parentFolderId,childFolderCount,totalItemCount,unreadItemCount"
const immutableIDPreference = "IdType=\"ImmutableId\""

type Service struct {
	client      *graph.Client
	appOnlyUser string
}

func New(client *graph.Client, appOnlyUser string) *Service {
	return &Service{client: client, appOnlyUser: strings.TrimSpace(appOnlyUser)}
}

func (s *Service) List(ctx context.Context, max int, queryText string, page string, folder string) ([]map[string]any, string, error) {
	query := url.Values{}
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", max))
	}
	query.Set("$select", messageListSelect)

	headers := mailReadHeaders()
	if strings.TrimSpace(queryText) != "" {
		query.Set("$search", fmt.Sprintf("\"%s\"", strings.TrimSpace(queryText)))
		headers.Set("ConsistencyLevel", "eventual")
	}

	endpoint := s.messagesEndpoint(folder)
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
		if hasSearchQuery(page) {
			headers.Set("ConsistencyLevel", "eventual")
		}
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listMailScopes, headers)
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	return items, next, nil
}

func (s *Service) ListFolders(ctx context.Context, max int, page string, includeHidden bool) ([]map[string]any, string, error) {
	query := url.Values{}
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", max))
	}
	query.Set("$select", mailFolderListSelect)
	if includeHidden {
		query.Set("includeHiddenFolders", "true")
	}

	endpoint := s.mailFoldersEndpoint()
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listMailFolderScopes, mailReadHeaders())
	if err != nil {
		return nil, "", err
	}

	items, next, err := graph.DecodeODataPage(body)
	if err != nil {
		return nil, "", err
	}

	return items, next, nil
}

func (s *Service) Get(ctx context.Context, id string) (map[string]any, error) {
	var payload map[string]any
	_, body, err := s.client.Do(
		ctx,
		http.MethodGet,
		s.messagesEndpoint("")+"/"+url.PathEscape(strings.TrimSpace(id)),
		nil,
		nil,
		getMailScopes,
		mailReadHeaders(),
	)
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("decode response json: %w", err)
	}
	return payload, nil
}

func (s *Service) Send(ctx context.Context, to []string, subject string, body string) error {
	toRecipients := make([]map[string]any, 0, len(to))
	for _, address := range to {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		toRecipients = append(toRecipients, map[string]any{
			"emailAddress": map[string]any{"address": address},
		})
	}

	payload := map[string]any{
		"message": map[string]any{
			"subject": subject,
			"body": map[string]any{
				"contentType": "Text",
				"content":     body,
			},
			"toRecipients": toRecipients,
		},
		"saveToSentItems": true,
	}

	_, _, err := s.client.Do(ctx, http.MethodPost, s.sendMailEndpoint(), nil, payload, sendMailScopes, nil)
	return err
}

func (s *Service) messagesEndpoint(folder string) string {
	root := s.mailboxEndpoint()
	if folder := strings.TrimSpace(folder); folder != "" {
		return root + "/mailFolders/" + url.PathEscape(folder) + "/messages"
	}
	return root + "/messages"
}

func (s *Service) mailFoldersEndpoint() string {
	return s.mailboxEndpoint() + "/mailFolders"
}

func (s *Service) mailboxEndpoint() string {
	if s.appOnlyUser != "" {
		return "/users/" + url.PathEscape(s.appOnlyUser)
	}
	return "/me"
}

func (s *Service) sendMailEndpoint() string {
	if s.appOnlyUser != "" {
		return "/users/" + url.PathEscape(s.appOnlyUser) + "/sendMail"
	}
	return "/me/sendMail"
}

func hasSearchQuery(page string) bool {
	u, err := url.Parse(strings.TrimSpace(page))
	if err != nil {
		return false
	}
	return strings.TrimSpace(u.Query().Get("$search")) != ""
}

func mailReadHeaders() http.Header {
	headers := http.Header{}
	headers.Set("Prefer", immutableIDPreference)
	return headers
}
