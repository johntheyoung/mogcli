package teams

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
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

func TestListBuildsJoinedTeamsEndpointWithoutTop(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/me/joinedTeams" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("$top") != "" {
			t.Fatalf("expected no $top query for joinedTeams, got %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("$select") == "" {
			t.Fatal("expected $select query")
		}
		_, _ = fmt.Fprint(w, `{"value":[{"id":"1"},{"id":"2"}]}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	items, next, err := New(client).List(context.Background(), 1, "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if next != "" {
		t.Fatalf("expected empty next link, got %q", next)
	}
	if len(items) != 1 || items[0]["id"] != "1" {
		t.Fatalf("unexpected items: %#v", items)
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
		if r.URL.Query().Get("$top") != "" {
			t.Fatalf("expected no $top query for channels, got %q", r.URL.RawQuery)
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

func TestChatMembersBuildsEndpointAndScopes(t *testing.T) {
	t.Parallel()

	var gotScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/chats/chat-id/messages" && r.URL.Path != "/chats/chat-id/members" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Path != "/chats/chat-id/members" {
			t.Fatalf("expected chat members path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "" {
			t.Fatalf("expected no query options, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprint(w, `{"value":[{"displayName":"Jane Doe","userId":"aad-user-1","email":"jane@example.com"}]}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(_ context.Context, scopes []string) (string, error) {
		gotScopes = scopes
		return "token", nil
	})
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	items, next, err := New(client).ChatMembers(context.Background(), "chat-id", 2, "")
	if err != nil {
		t.Fatalf("ChatMembers failed: %v", err)
	}
	if next != "" {
		t.Fatalf("unexpected next: %s", next)
	}
	if len(items) != 1 || items[0]["userId"] != "aad-user-1" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if len(gotScopes) != 1 || gotScopes[0] != "ChatMember.Read" {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
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

func TestChatsBuildsMeChatsEndpointAndScopes(t *testing.T) {
	t.Parallel()

	var gotScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/me/chats" {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if r.URL.Query().Get("$top") != "50" {
			t.Fatalf("expected capped $top=50, got %q", r.URL.RawQuery)
		}
		if r.URL.Query().Get("$select") == "" {
			t.Fatal("expected $select query")
		}
		_, _ = fmt.Fprint(w, `{"value":[{"id":"chat-id","chatType":"oneOnOne"}]}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(_ context.Context, scopes []string) (string, error) {
		gotScopes = scopes
		return "token", nil
	})
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	items, next, err := New(client).Chats(context.Background(), 100, "")
	if err != nil {
		t.Fatalf("Chats failed: %v", err)
	}
	if next != "" {
		t.Fatalf("expected empty next link, got %q", next)
	}
	if len(items) != 1 || items[0]["id"] != "chat-id" {
		t.Fatalf("unexpected items: %#v", items)
	}
	if len(gotScopes) != 1 || gotScopes[0] != "Chat.ReadBasic" {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
	}
}

func TestChatsUsesPageTokenURL(t *testing.T) {
	t.Parallel()

	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chats/next" {
			t.Fatalf("expected /chats/next path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "state=abc" {
			t.Fatalf("expected resume query, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/chats/next?state=next"}`, serverURL)
	}))
	defer server.Close()
	serverURL = server.URL

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = serverURL
	client.HTTPClient = server.Client()

	items, next, err := New(client).Chats(context.Background(), 1, serverURL+"/chats/next?state=abc")
	if err != nil {
		t.Fatalf("Chats failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item due to max cap, got %d", len(items))
	}
	if next != serverURL+"/chats/next?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestSendChatMessagePostsPayloadAndScopes(t *testing.T) {
	t.Parallel()

	var gotScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/chats/chat-id/messages" {
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
		if _, ok := payload["mentions"]; ok {
			t.Fatalf("expected no mentions for plain SendChatMessage, got %#v", payload["mentions"])
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

	item, err := New(client).SendChatMessage(context.Background(), "chat-id", " <b>Hello</b> ", "html")
	if err != nil {
		t.Fatalf("SendChatMessage failed: %v", err)
	}
	if item["id"] != "message-id" {
		t.Fatalf("unexpected response item: %#v", item)
	}
	if len(gotScopes) != 1 || gotScopes[0] != "ChatMessage.Send" {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
	}
}

func TestSendChatMessageWithMentionsPostsGraphMentionPayload(t *testing.T) {
	t.Parallel()

	var gotScopes []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method %s", r.Method)
		}
		if r.URL.Path != "/chats/chat-id/messages" {
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
		wantContent := `Hello &lt;team&gt;<br><at id="0">Jane Doe</at> <at id="1">John Smith</at>`
		if body["content"] != wantContent {
			t.Fatalf("unexpected content:\ngot:  %#v\nwant: %#v", body["content"], wantContent)
		}

		mentions, ok := payload["mentions"].([]any)
		if !ok || len(mentions) != 2 {
			t.Fatalf("unexpected mentions: %#v", payload["mentions"])
		}
		first, ok := mentions[0].(map[string]any)
		if !ok {
			t.Fatalf("unexpected first mention: %#v", mentions[0])
		}
		if first["id"] != float64(0) {
			t.Fatalf("unexpected first id: %#v", first["id"])
		}
		if first["mentionText"] != "Jane Doe" {
			t.Fatalf("unexpected first mentionText: %#v", first["mentionText"])
		}
		mentioned, ok := first["mentioned"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected mentioned payload: %#v", first["mentioned"])
		}
		user, ok := mentioned["user"].(map[string]any)
		if !ok {
			t.Fatalf("unexpected mentioned user: %#v", mentioned["user"])
		}
		if user["displayName"] != "Jane Doe" || user["id"] != "aad-user-1" || user["userIdentityType"] != "aadUser" {
			t.Fatalf("unexpected mentioned user payload: %#v", user)
		}

		second, ok := mentions[1].(map[string]any)
		if !ok {
			t.Fatalf("unexpected second mention: %#v", mentions[1])
		}
		if second["id"] != float64(1) || second["mentionText"] != "John Smith" {
			t.Fatalf("unexpected second mention payload: %#v", second)
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

	item, err := New(client).SendChatMessageWithMentions(context.Background(), "chat-id", " Hello <team> ", "text", []ChatMention{
		{DisplayName: "Jane Doe", UserID: "aad-user-1"},
		{DisplayName: "John Smith", UserID: "aad-user-2"},
	})
	if err != nil {
		t.Fatalf("SendChatMessageWithMentions failed: %v", err)
	}
	if item["id"] != "message-id" {
		t.Fatalf("unexpected response item: %#v", item)
	}
	if len(gotScopes) != 1 || gotScopes[0] != "ChatMessage.Send" {
		t.Fatalf("unexpected scopes: %#v", gotScopes)
	}
}

func TestOpenOneOnOneChatUsesMeThenCreatesChat(t *testing.T) {
	t.Parallel()

	var gotScopes [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/me":
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method for /me: %s", r.Method)
			}
			_, _ = fmt.Fprint(w, `{"id":"current-user-id","userPrincipalName":"me@contoso.com"}`)
		case "/chats":
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method for /chats: %s", r.Method)
			}

			var payload map[string]any
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode body failed: %v", err)
			}
			if payload["chatType"] != "oneOnOne" {
				t.Fatalf("unexpected chatType: %#v", payload["chatType"])
			}
			members, ok := payload["members"].([]any)
			if !ok || len(members) != 2 {
				t.Fatalf("unexpected members: %#v", payload["members"])
			}
			first, ok := members[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected first member: %#v", members[0])
			}
			second, ok := members[1].(map[string]any)
			if !ok {
				t.Fatalf("unexpected second member: %#v", members[1])
			}
			if first["user@odata.bind"] != graphUserBinding("current-user-id") {
				t.Fatalf("unexpected current user bind: %#v", first["user@odata.bind"])
			}
			if second["user@odata.bind"] != graphUserBinding("target@contoso.com") {
				t.Fatalf("unexpected target user bind: %#v", second["user@odata.bind"])
			}

			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":"chat-id"}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := graph.NewClient(func(_ context.Context, scopes []string) (string, error) {
		gotScopes = append(gotScopes, append([]string(nil), scopes...))
		return "token", nil
	})
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()

	item, err := New(client).OpenOneOnOneChat(context.Background(), "target@contoso.com")
	if err != nil {
		t.Fatalf("OpenOneOnOneChat failed: %v", err)
	}
	if item["id"] != "chat-id" {
		t.Fatalf("unexpected response item: %#v", item)
	}
	if len(gotScopes) != 2 {
		t.Fatalf("expected two token requests, got %#v", gotScopes)
	}
	if len(gotScopes[0]) != 1 || gotScopes[0][0] != "User.Read" {
		t.Fatalf("unexpected /me scopes: %#v", gotScopes[0])
	}
	if len(gotScopes[1]) != 1 || gotScopes[1][0] != "Chat.Create" {
		t.Fatalf("unexpected /chats scopes: %#v", gotScopes[1])
	}
}

func TestTypedChatVerificationReadsAndReferenceAttachmentPayload(t *testing.T) {
	t.Parallel()

	var gotMessagePayload map[string]any
	var gotScopes [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/me":
			if got := r.URL.Query().Get("$select"); got != "id,displayName,mail,userPrincipalName,userType" {
				t.Fatalf("unexpected /me select: %q", got)
			}
			_, _ = fmt.Fprint(w, `{"id":"self-id","displayName":"Self","userType":"Member"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/chats/chat-id":
			if r.URL.RawQuery != "" {
				t.Fatalf("get chat does not support OData query parameters, got %q", r.URL.RawQuery)
			}
			_, _ = fmt.Fprint(w, `{"id":"chat-id","chatType":"oneOnOne"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/chats/chat-id/members":
			_, _ = fmt.Fprint(w, `{"value":[{"@odata.type":"#microsoft.graph.aadUserConversationMember","userId":"self-id","tenantId":"tenant-id","roles":["owner"]},{"@odata.type":"#microsoft.graph.aadUserConversationMember","userId":"recipient-id","tenantId":"tenant-id","roles":["owner"]}]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/chats/chat-id/messages":
			if err := json.NewDecoder(r.Body).Decode(&gotMessagePayload); err != nil {
				t.Fatalf("decode message payload: %v", err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":"message-id"}`)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()

	client := graph.NewClient(func(_ context.Context, scopes []string) (string, error) {
		gotScopes = append(gotScopes, append([]string(nil), scopes...))
		return "token", nil
	})
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()
	svc := New(client)

	current, err := svc.CurrentUser(context.Background())
	if err != nil || current.ID != "self-id" {
		t.Fatalf("CurrentUser failed: current=%#v err=%v", current, err)
	}
	chat, err := svc.Chat(context.Background(), "chat-id")
	if err != nil || chat.ChatType != "oneOnOne" {
		t.Fatalf("Chat failed: chat=%#v err=%v", chat, err)
	}
	members, next, err := svc.ChatMembersForVerification(context.Background(), "chat-id")
	if err != nil || next != "" || len(members) != 2 {
		t.Fatalf("ChatMembersForVerification failed: members=%#v next=%q err=%v", members, next, err)
	}
	message, err := svc.SendReferenceAttachment(context.Background(), "chat-id", "Hi <all> &\nsecond", ReferenceAttachment{
		ID:         "item-id",
		ContentURL: "https://contoso.example/item?a=1&b=2",
		Name:       `report & "notes".txt`,
	})
	if err != nil || message.ID != "message-id" {
		t.Fatalf("SendReferenceAttachment failed: message=%#v err=%v", message, err)
	}

	body, ok := gotMessagePayload["body"].(map[string]any)
	if !ok {
		t.Fatalf("unexpected body: %#v", gotMessagePayload["body"])
	}
	wantHTML := `Hi &lt;all&gt; &amp;<br>second<br><attachment id="item-id"></attachment>`
	if body["contentType"] != "html" || body["content"] != wantHTML {
		t.Fatalf("unexpected message body: %#v", body)
	}
	attachments, ok := gotMessagePayload["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected one attachment, got %#v", gotMessagePayload["attachments"])
	}
	attachment, ok := attachments[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected attachment: %#v", attachments[0])
	}
	if attachment["id"] != "item-id" ||
		attachment["contentType"] != "reference" ||
		attachment["contentUrl"] != "https://contoso.example/item?a=1&b=2" ||
		attachment["name"] != `report & "notes".txt` {
		t.Fatalf("unexpected attachment payload: %#v", attachment)
	}
	wantScopes := [][]string{
		{"User.Read"},
		{"Chat.ReadBasic"},
		{"ChatMember.Read"},
		{"ChatMessage.Send"},
	}
	if !reflect.DeepEqual(gotScopes, wantScopes) {
		t.Fatalf("unexpected least-privilege request scopes: got %#v want %#v", gotScopes, wantScopes)
	}
}

func TestReferenceAttachmentSendDoesNotRetry(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"error":{"code":"busy","message":"try later"}}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()
	client.MaxRetries5xx = 3

	_, err := New(client).SendReferenceAttachment(context.Background(), "chat-id", "", ReferenceAttachment{
		ID:         "item-id",
		ContentURL: "https://contoso.example/item",
		Name:       "report.txt",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if requests != 1 {
		t.Fatalf("final send must not retry; got %d requests", requests)
	}
}
