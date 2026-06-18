package teams

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
		if r.URL.Path != "/teams/next" {
			t.Fatalf("expected /teams/next path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "state=abc" {
			t.Fatalf("expected resume query, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/teams/next?state=next"}`, serverURL)
	}))
	defer server.Close()
	serverURL = server.URL

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = serverURL
	client.HTTPClient = server.Client()

	svc := New(client)
	items, next, err := svc.List(context.Background(), 1, serverURL+"/teams/next?state=abc")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item due to max cap, got %d", len(items))
	}
	if next != serverURL+"/teams/next?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestChannelsUsesTeamEndpointAndPageTokenURL(t *testing.T) {
	t.Parallel()

	requests := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/channels/next" {
			t.Fatalf("expected /channels/next path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "state=abc" {
			t.Fatalf("expected resume query, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/channels/next?state=next"}`, serverURL)
	}))
	defer server.Close()
	serverURL = server.URL

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = serverURL
	client.HTTPClient = server.Client()

	svc := New(client)
	items, next, err := svc.Channels(context.Background(), "ignored-team-id", 1, serverURL+"/channels/next?state=abc")
	if err != nil {
		t.Fatalf("Channels failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item due to max cap, got %d", len(items))
	}
	if next != serverURL+"/channels/next?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestChannelsBuildsTeamEndpoint(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/teams/team-id/channels" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("$select") == "" {
			t.Fatal("expected $select query")
		}
		_, _ = fmt.Fprint(w, `{"value":[{"id":"channel-id"}]}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	items, next, err := New(client).Channels(context.Background(), "team-id", 10, "")
	if err != nil {
		t.Fatalf("Channels failed: %v", err)
	}
	if next != "" {
		t.Fatalf("expected empty next link, got %q", next)
	}
	if len(items) != 1 || items[0]["id"] != "channel-id" {
		t.Fatalf("unexpected items: %#v", items)
	}
}

func TestSendChannelMessagePostsPayloadAndScopes(t *testing.T) {
	t.Parallel()

	var gotScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/teams/team-id/channels/channel-id/messages" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		body, ok := payload["body"].(map[string]any)
		if !ok {
			t.Fatalf("expected body map, got %#v", payload["body"])
		}
		if body["contentType"] != "html" {
			t.Fatalf("unexpected contentType: %#v", body["contentType"])
		}
		if body["content"] != "<b>Hello</b>" {
			t.Fatalf("unexpected content: %#v", body["content"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id":"message-id"}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(_ context.Context, scopes []string) (string, error) {
		gotScopes = scopes
		return "token", nil
	})
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	item, err := New(client).SendChannelMessage(context.Background(), "team-id", "channel-id", " <b>Hello</b> ", "html")
	if err != nil {
		t.Fatalf("SendChannelMessage failed: %v", err)
	}
	if item["id"] != "message-id" {
		t.Fatalf("unexpected response item: %#v", item)
	}
	if len(gotScopes) != 1 || gotScopes[0] != "ChannelMessage.Send" {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
	}
}
