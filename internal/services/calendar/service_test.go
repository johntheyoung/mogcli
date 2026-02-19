package calendar

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

func TestListUsesPageTokenURL(t *testing.T) {
	t.Parallel()

	requests := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/calendar/next" {
			t.Fatalf("expected /calendar/next path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "state=abc" {
			t.Fatalf("expected resume query, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/calendar/next?state=next"}`, serverURL)
	}))
	defer server.Close()
	serverURL = server.URL

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = serverURL
	client.HTTPClient = server.Client()

	svc := New(client)
	items, next, err := svc.List(context.Background(), "2026-02-12T00:00:00Z", "2026-02-13T00:00:00Z", 1, serverURL+"/calendar/next?state=abc")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item due to max cap, got %d", len(items))
	}
	if next != serverURL+"/calendar/next?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestUpdateSplitsAttendeesFromReminderPatch(t *testing.T) {
	var payloads []map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/me/events/event-id" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		payloads = append(payloads, body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	payload := map[string]any{
		"subject":                    "Updated",
		"isReminderOn":               true,
		"reminderMinutesBeforeStart": 15,
		"attendees": []map[string]any{
			{
				"emailAddress": map[string]any{"address": "dev@example.com"},
				"type":         "required",
			},
		},
	}

	if err := New(client).Update(context.Background(), "event-id", payload); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if len(payloads) != 2 {
		t.Fatalf("expected two patch requests, got %d", len(payloads))
	}

	if _, ok := payloads[0]["attendees"]; ok {
		t.Fatalf("expected first patch payload to exclude attendees: %#v", payloads[0])
	}
	if _, ok := payloads[0]["isReminderOn"]; !ok {
		t.Fatalf("expected first payload to include reminder field: %#v", payloads[0])
	}

	if len(payloads[1]) != 1 {
		t.Fatalf("expected second payload to include only attendees, got %#v", payloads[1])
	}
	if _, ok := payloads[1]["attendees"]; !ok {
		t.Fatalf("expected second payload to include attendees: %#v", payloads[1])
	}
}

func TestUpdateSendsSinglePatchWhenNoReminderAttendeeConflict(t *testing.T) {
	requests := 0
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPatch {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/me/events/event-id" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	payload := map[string]any{
		"subject": "Updated",
		"attendees": []map[string]any{
			{
				"emailAddress": map[string]any{"address": "dev@example.com"},
				"type":         "required",
			},
		},
	}

	if err := New(client).Update(context.Background(), "event-id", payload); err != nil {
		t.Fatalf("Update failed: %v", err)
	}

	if requests != 1 {
		t.Fatalf("expected one patch request, got %d", requests)
	}
	if _, ok := received["attendees"]; !ok {
		t.Fatalf("expected attendees field in patch payload: %#v", received)
	}
}
