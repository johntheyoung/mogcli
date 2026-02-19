package calendar

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listCalendarScopes = []string{"Calendars.Read"}
var getCalendarScopes = []string{"Calendars.Read"}
var createCalendarScopes = []string{"Calendars.ReadWrite"}
var updateCalendarScopes = []string{"Calendars.ReadWrite"}
var deleteCalendarScopes = []string{"Calendars.ReadWrite"}

type Service struct {
	client *graph.Client
}

func New(client *graph.Client) *Service {
	return &Service{client: client}
}

func (s *Service) List(ctx context.Context, from string, to string, max int, page string) ([]map[string]any, string, error) {
	query := url.Values{}
	query.Set("startDateTime", from)
	query.Set("endDateTime", to)
	query.Set("$select", "id,subject,start,end,organizer")
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", max))
	}

	headers := http.Header{}
	headers.Set("Prefer", `outlook.timezone="UTC"`)

	endpoint := "/me/calendarView"
	if strings.TrimSpace(page) != "" {
		endpoint = strings.TrimSpace(page)
		query = nil
	}

	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, query, nil, listCalendarScopes, headers)
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

func (s *Service) Get(ctx context.Context, id string) (map[string]any, error) {
	var payload map[string]any
	err := s.client.DoJSON(ctx, http.MethodGet, "/me/events/"+url.PathEscape(strings.TrimSpace(id)), nil, nil, getCalendarScopes, &payload)
	return payload, err
}

func (s *Service) Create(ctx context.Context, payload map[string]any) (map[string]any, error) {
	var created map[string]any
	err := s.client.DoJSON(ctx, http.MethodPost, "/me/events", nil, payload, createCalendarScopes, &created)
	return created, err
}

func (s *Service) Update(ctx context.Context, id string, payload map[string]any) error {
	endpoint := "/me/events/" + url.PathEscape(strings.TrimSpace(id))
	primary, attendeesOnly := splitUpdatePayload(payload)

	if len(primary) > 0 {
		if _, _, err := s.client.Do(ctx, http.MethodPatch, endpoint, nil, primary, updateCalendarScopes, nil); err != nil {
			return err
		}
	}

	if len(attendeesOnly) > 0 {
		if _, _, err := s.client.Do(ctx, http.MethodPatch, endpoint, nil, attendeesOnly, updateCalendarScopes, nil); err != nil {
			return err
		}
	}

	return nil
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, _, err := s.client.Do(ctx, http.MethodDelete, "/me/events/"+url.PathEscape(strings.TrimSpace(id)), nil, nil, deleteCalendarScopes, nil)
	return err
}

func trimPage(items []map[string]any, next string, max int) ([]map[string]any, string) {
	if max <= 0 || len(items) <= max {
		return items, next
	}
	return items[:max], next
}

func splitUpdatePayload(payload map[string]any) (map[string]any, map[string]any) {
	if len(payload) == 0 {
		return map[string]any{}, nil
	}

	attendees, hasAttendees := payload["attendees"]
	if !hasAttendees || !hasReminderField(payload) {
		return clonePayload(payload), nil
	}

	primary := clonePayload(payload)
	delete(primary, "attendees")
	return primary, map[string]any{"attendees": attendees}
}

func hasReminderField(payload map[string]any) bool {
	_, hasReminderOn := payload["isReminderOn"]
	_, hasReminderMins := payload["reminderMinutesBeforeStart"]
	return hasReminderOn || hasReminderMins
}

func clonePayload(payload map[string]any) map[string]any {
	cloned := make(map[string]any, len(payload))
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}
