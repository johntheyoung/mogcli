package mail

import (
	"context"
	"encoding/json"
	"errors"
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
var mutateMailScopes = []string{"Mail.ReadWrite"}

const messageListSelect = "id,parentFolderId,conversationId,subject,from,sender,toRecipients,ccRecipients,receivedDateTime,sentDateTime,isRead,isDraft,hasAttachments,importance,flag,categories,inferenceClassification,webLink"
const mailFolderListSelect = "id,displayName,parentFolderId,childFolderCount,totalItemCount,unreadItemCount"
const immutableIDPreference = "IdType=\"ImmutableId\""
const archiveWellKnownFolder = "archive"

// IndeterminateMoveError means mog cannot safely tell whether Graph committed a
// non-idempotent move. Callers must inspect mailbox state instead of retrying
// automatically.
type IndeterminateMoveError struct {
	Err error
}

func (e *IndeterminateMoveError) Error() string {
	return fmt.Sprintf("mail move outcome is indeterminate; do not retry automatically: %v", e.Err)
}

func (e *IndeterminateMoveError) Unwrap() error {
	return e.Err
}

// ConfirmedMoveResponseError means Graph returned HTTP 201, so the move was
// committed, but mog could not use the returned message representation.
type ConfirmedMoveResponseError struct {
	Err error
}

func (e *ConfirmedMoveResponseError) Error() string {
	return fmt.Sprintf("mail move was confirmed with HTTP 201, but its response could not be used: %v", e.Err)
}

func (e *ConfirmedMoveResponseError) Unwrap() error {
	return e.Err
}

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

// Archive moves one explicit message to Outlook's documented archive
// well-known folder.
func (s *Service) Archive(ctx context.Context, id string) (map[string]any, error) {
	return s.Move(ctx, id, archiveWellKnownFolder)
}

// Move moves one explicit message to one explicit destination. Graph implements
// this as copy-then-remove and returns HTTP 201 with the new message resource,
// so automatic retries are disabled.
func (s *Service) Move(ctx context.Context, id string, destination string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	destination = strings.TrimSpace(destination)
	if id == "" {
		return nil, fmt.Errorf("message id is required")
	}
	if destination == "" {
		return nil, fmt.Errorf("destination folder is required")
	}

	endpoint := s.messagesEndpoint("") + "/" + url.PathEscape(id) + "/move"
	resp, body, err := s.client.DoWithOptions(
		ctx,
		http.MethodPost,
		endpoint,
		nil,
		map[string]any{"destinationId": destination},
		mutateMailScopes,
		mailReadHeaders(),
		graph.RequestOptions{DisableRetries: true},
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusCreated {
			return nil, &ConfirmedMoveResponseError{Err: err}
		}
		if resp != nil && resp.StatusCode >= http.StatusBadRequest && resp.StatusCode < http.StatusInternalServerError {
			return nil, fmt.Errorf("Graph rejected the move with HTTP %d: %w", resp.StatusCode, err)
		}
		if moveOutcomeIsIndeterminate(resp, err) {
			return nil, &IndeterminateMoveError{Err: err}
		}
		return nil, err
	}
	if resp == nil || resp.StatusCode != http.StatusCreated {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, &IndeterminateMoveError{
			Err: fmt.Errorf("Graph did not confirm the move with HTTP 201 (status %d)", status),
		}
	}

	var moved map[string]any
	if err := json.Unmarshal(body, &moved); err != nil {
		return nil, &ConfirmedMoveResponseError{Err: fmt.Errorf("decode message response: %w", err)}
	}
	if strings.TrimSpace(mapString(moved, "id")) == "" {
		return nil, &ConfirmedMoveResponseError{Err: fmt.Errorf("message response did not include an id")}
	}
	return moved, nil
}

// MarkRead sets isRead=true on one explicit message and returns Graph's updated
// message representation.
func (s *Service) MarkRead(ctx context.Context, id string) (map[string]any, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, fmt.Errorf("message id is required")
	}

	endpoint := s.messagesEndpoint("") + "/" + url.PathEscape(id)
	resp, body, err := s.client.Do(
		ctx,
		http.MethodPatch,
		endpoint,
		nil,
		map[string]any{"isRead": true},
		mutateMailScopes,
		mailReadHeaders(),
	)
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusOK {
			return nil, fmt.Errorf("message read-state update was confirmed with HTTP 200, but its response could not be read: %w", err)
		}
		return nil, err
	}
	if resp == nil || resp.StatusCode != http.StatusOK {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return nil, fmt.Errorf("Graph did not confirm the message read-state update with HTTP 200 (status %d)", status)
	}

	var updated map[string]any
	if err := json.Unmarshal(body, &updated); err != nil {
		return nil, fmt.Errorf("decode updated message response: %w", err)
	}
	if isRead, ok := updated["isRead"].(bool); !ok || !isRead {
		return nil, fmt.Errorf("updated message response did not confirm isRead=true")
	}
	return updated, nil
}

func moveOutcomeIsIndeterminate(resp *http.Response, err error) bool {
	if resp != nil && resp.StatusCode >= http.StatusInternalServerError {
		return true
	}

	var transportErr *graph.TransportError
	if errors.As(err, &transportErr) {
		return true
	}
	var responseReadErr *graph.ResponseReadError
	return errors.As(err, &responseReadErr)
}

func mapString(item map[string]any, key string) string {
	value, _ := item[key].(string)
	return value
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
