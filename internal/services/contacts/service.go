package contacts

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listContactsScopes = []string{"Contacts.Read"}
var getContactsScopes = []string{"Contacts.Read"}
var createContactsScopes = []string{"Contacts.ReadWrite"}
var updateContactsScopes = []string{"Contacts.ReadWrite"}
var deleteContactsScopes = []string{"Contacts.ReadWrite"}

type Service struct {
	client      *graph.Client
	appOnlyUser string
}

func New(client *graph.Client, appOnlyUser string) *Service {
	return &Service{client: client, appOnlyUser: strings.TrimSpace(appOnlyUser)}
}

func (s *Service) List(ctx context.Context, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("$select", "id,displayName,emailAddresses,mobilePhone,companyName,jobTitle,businessHomePage,personalNotes,categories")
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", max))
	}

	endpoint := s.contactsEndpoint()
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listContactsScopes, nil)
	if err != nil {
		return nil, "", err
	}

	items, next, err := decodeValuePage(body)
	if err != nil {
		return nil, "", err
	}

	trimmed, trimmedNext := trimPage(items, next, max)
	for _, item := range trimmed {
		addNormalizedCustomFields(item)
	}
	return trimmed, trimmedNext, nil
}

func (s *Service) Get(ctx context.Context, id string) (map[string]any, error) {
	var payload map[string]any
	err := s.client.DoJSON(ctx, http.MethodGet, s.contactsEndpoint()+"/"+url.PathEscape(strings.TrimSpace(id)), nil, nil, getContactsScopes, &payload)
	addNormalizedCustomFields(payload)
	return payload, err
}

func (s *Service) Create(ctx context.Context, payload map[string]any) (map[string]any, error) {
	var created map[string]any
	err := s.client.DoJSON(ctx, http.MethodPost, s.contactsEndpoint(), nil, payload, createContactsScopes, &created)
	addNormalizedCustomFields(created)
	return created, err
}

func (s *Service) Update(ctx context.Context, id string, payload map[string]any) error {
	_, _, err := s.client.Do(ctx, http.MethodPatch, s.contactsEndpoint()+"/"+url.PathEscape(strings.TrimSpace(id)), nil, payload, updateContactsScopes, nil)
	return err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, _, err := s.client.Do(ctx, http.MethodDelete, s.contactsEndpoint()+"/"+url.PathEscape(strings.TrimSpace(id)), nil, nil, deleteContactsScopes, nil)
	return err
}

func (s *Service) contactsEndpoint() string {
	if s.appOnlyUser != "" {
		return "/users/" + url.PathEscape(s.appOnlyUser) + "/contacts"
	}
	return "/me/contacts"
}

func decodeValuePage(body []byte) ([]map[string]any, string, error) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, "", fmt.Errorf("decode json response: %w", err)
	}

	items := make([]map[string]any, 0)
	if values, ok := payload["value"].([]any); ok {
		for _, value := range values {
			if item, ok := value.(map[string]any); ok {
				items = append(items, item)
			}
		}
	}

	next, _ := payload["@odata.nextLink"].(string)
	return items, strings.TrimSpace(next), nil
}

func trimPage(items []map[string]any, next string, max int) ([]map[string]any, string) {
	if max <= 0 || len(items) <= max {
		return items, next
	}
	return items[:max], next
}
