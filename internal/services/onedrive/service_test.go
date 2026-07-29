package onedrive

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

func TestListUsesPageTokenURL(t *testing.T) {
	t.Parallel()

	requests := 0
	var serverURL string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/resume" {
			t.Fatalf("expected /resume path, got %s", r.URL.Path)
		}
		if r.URL.RawQuery != "state=abc" {
			t.Fatalf("expected resume query, got %q", r.URL.RawQuery)
		}
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/resume?state=next"}`, serverURL)
	}))
	defer server.Close()
	serverURL = server.URL

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = serverURL
	client.HTTPClient = server.Client()

	svc := New(client, "")
	items, next, err := svc.List(context.Background(), "/ignored", 1, serverURL+"/resume?state=abc")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if len(items) != 1 {
		t.Fatalf("expected one item due to max cap, got %d", len(items))
	}
	if next != serverURL+"/resume?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestEndpointsRouteByAuthMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name       string
		user       string
		basePrefix string
	}{
		{
			name:       "delegated uses me endpoints",
			user:       "",
			basePrefix: "/me/drive",
		},
		{
			name:       "app-only uses user endpoints",
			user:       "person@example.com",
			basePrefix: "/users/" + url.PathEscape("person@example.com") + "/drive",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == tc.basePrefix+"/root/children":
					_, _ = fmt.Fprint(w, `{"value":[]}`)
				case r.Method == http.MethodGet && r.URL.Path == tc.basePrefix+"/root:/docs/report.txt:/content":
					_, _ = fmt.Fprint(w, `hello`)
				case r.Method == http.MethodGet && r.URL.Path == tc.basePrefix+"/root:/docs/report.txt":
					_, _ = fmt.Fprint(w, `{"id":"item-id","size":5}`)
				case r.Method == http.MethodPut && r.URL.Path == tc.basePrefix+"/root:/docs/report.txt:/content":
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodPost && r.URL.Path == tc.basePrefix+"/root:/projects:/children":
					w.WriteHeader(http.StatusCreated)
				case r.Method == http.MethodDelete && r.URL.Path == tc.basePrefix+"/root:/projects/old":
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()

			client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
			client.BaseURL = server.URL
			client.HTTPClient = server.Client()

			svc := New(client, tc.user)
			if _, _, err := svc.List(context.Background(), "/", 10, ""); err != nil {
				t.Fatalf("List failed: %v", err)
			}
			if _, err := svc.Get(context.Background(), "/docs/report.txt"); err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if _, err := svc.Stat(context.Background(), "/docs/report.txt"); err != nil {
				t.Fatalf("Stat failed: %v", err)
			}
			if err := svc.Put(context.Background(), "/docs/report.txt", []byte("hello")); err != nil {
				t.Fatalf("Put failed: %v", err)
			}
			if err := svc.Mkdir(context.Background(), "/projects/new"); err != nil {
				t.Fatalf("Mkdir failed: %v", err)
			}
			if err := svc.Remove(context.Background(), "/projects/old"); err != nil {
				t.Fatalf("Remove failed: %v", err)
			}
		})
	}
}

func TestFileSharingPrimitivesUseSafePayloadsAndItemIDs(t *testing.T) {
	t.Parallel()

	var invitePayload map[string]any
	var gotScopes [][]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/me/drive/special/approot":
			_, _ = fmt.Fprint(w, `{"id":"app-root-id","name":"Mog"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/me/drive/items/app-root-id:/mog-chat-file-fixed:/content":
			if got := r.URL.Query().Get("@microsoft.graph.conflictBehavior"); got != "fail" {
				t.Fatalf("expected conflict behavior fail, got %q", got)
			}
			content, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read upload failed: %v", err)
			}
			if string(content) != "hello" {
				t.Fatalf("unexpected upload content %q", content)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = fmt.Fprint(w, `{"id":"item-id","name":"mog-chat-file-fixed","size":5,"webUrl":"https://contoso.example/item"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/me/drive/items/item-id/content":
			_, _ = fmt.Fprint(w, "hello")
		case r.Method == http.MethodPost && r.URL.Path == "/me/drive/items/item-id/invite":
			if err := json.NewDecoder(r.Body).Decode(&invitePayload); err != nil {
				t.Fatalf("decode invite failed: %v", err)
			}
			_, _ = fmt.Fprint(w, `{"value":[{"id":"permission-id","roles":["read"],"grantedToV2":{"user":{"id":"recipient-id"}}}]}`)
		case r.Method == http.MethodGet && r.URL.Path == "/me/drive/items/item-id/permissions":
			_, _ = fmt.Fprint(w, `{"value":[{"id":"permission-id","roles":["read"],"grantedToV2":{"user":{"id":"recipient-id"}}}]}`)
		case r.Method == http.MethodDelete && r.URL.Path == "/me/drive/items/item-id/permissions/permission-id":
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && r.URL.Path == "/me/drive/items/item-id":
			w.WriteHeader(http.StatusNoContent)
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
	svc := New(client, "")

	root, err := svc.AppRoot(context.Background())
	if err != nil {
		t.Fatalf("AppRoot failed: %v", err)
	}
	item, err := svc.UploadSimpleFail(context.Background(), root.ID, "mog-chat-file-fixed", []byte("hello"))
	if err != nil {
		t.Fatalf("UploadSimpleFail failed: %v", err)
	}
	if item.ID != "item-id" {
		t.Fatalf("unexpected item: %#v", item)
	}
	content, err := svc.ContentByID(context.Background(), item.ID)
	if err != nil || string(content) != "hello" {
		t.Fatalf("ContentByID failed: content=%q err=%v", content, err)
	}
	invited, err := svc.InviteRead(context.Background(), item.ID, "recipient-id")
	if err != nil || len(invited) != 1 || invited[0].ID != "permission-id" {
		t.Fatalf("InviteRead failed: invited=%#v err=%v", invited, err)
	}
	recipients, ok := invitePayload["recipients"].([]any)
	if !ok || len(recipients) != 1 {
		t.Fatalf("unexpected recipients payload: %#v", invitePayload["recipients"])
	}
	recipient, ok := recipients[0].(map[string]any)
	if !ok || recipient["objectId"] != "recipient-id" {
		t.Fatalf("invite must use immutable objectId: %#v", recipients[0])
	}
	if invitePayload["requireSignIn"] != true ||
		invitePayload["sendInvitation"] != false ||
		invitePayload["retainInheritedPermissions"] != false {
		t.Fatalf("unexpected invite safety flags: %#v", invitePayload)
	}
	roles, ok := invitePayload["roles"].([]any)
	if !ok || len(roles) != 1 || roles[0] != "read" {
		t.Fatalf("unexpected roles: %#v", invitePayload["roles"])
	}
	permissions, next, err := svc.Permissions(context.Background(), item.ID)
	if err != nil || next != "" || len(permissions) != 1 {
		t.Fatalf("Permissions failed: permissions=%#v next=%q err=%v", permissions, next, err)
	}
	if err := svc.RemovePermission(context.Background(), item.ID, "permission-id"); err != nil {
		t.Fatalf("RemovePermission failed: %v", err)
	}
	if err := svc.RemoveByID(context.Background(), item.ID); err != nil {
		t.Fatalf("RemoveByID failed: %v", err)
	}
	for _, scopes := range gotScopes {
		if len(scopes) != 1 || scopes[0] != "Files.ReadWrite" {
			t.Fatalf("file sharing primitive requested unexpected scopes: %#v", gotScopes)
		}
	}
}

func TestInviteReadDoesNotRetryServerErrors(t *testing.T) {
	t.Parallel()

	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/invite") {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"error":{"code":"busy","message":"try later"}}`)
	}))
	defer server.Close()

	client := graph.NewClient(func(context.Context, []string) (string, error) { return "token", nil })
	client.BaseURL = server.URL
	client.HTTPClient = server.Client()
	client.MaxRetries5xx = 3

	_, err := New(client, "").InviteRead(context.Background(), "item-id", "recipient-id")
	if err == nil {
		t.Fatal("expected invite error")
	}
	if requests != 1 {
		t.Fatalf("invite must not retry; got %d requests", requests)
	}
}
