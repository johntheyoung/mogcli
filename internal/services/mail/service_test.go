package mail

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

const graphTestBaseURL = "https://graph.microsoft.com"

func TestListInitialFolderPageRequestsRichMetadata(t *testing.T) {
	t.Parallel()

	serverURL := graphTestBaseURL
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/me/mailFolders/inbox/messages" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("$top"); got != "2" {
			t.Fatalf("expected $top=2, got %q", got)
		}
		if got := r.URL.Query().Get("$select"); got != messageListSelect {
			t.Fatalf("unexpected $select: %q", got)
		}
		if got := r.URL.Query().Get("$search"); got != `"from:alerts@example.com"` {
			t.Fatalf("unexpected $search: %q", got)
		}
		if got := r.Header.Get("ConsistencyLevel"); got != "eventual" {
			t.Fatalf("expected ConsistencyLevel eventual, got %q", got)
		}
		assertImmutableIDPreference(t, r)
		_, _ = fmt.Fprintf(
			w,
			`{"value":[{"id":"1","parentFolderId":"inbox-id","conversationId":"conversation-1","subject":"Alert","isRead":false}],"@odata.nextLink":"%s/mail/next?state=next"}`,
			serverURL,
		)
	})

	var requestedScopes []string
	client := newMailTestClient(func(_ context.Context, scopes []string) (string, error) {
		requestedScopes = append([]string(nil), scopes...)
		return "token", nil
	}, handler)

	items, next, err := New(client, "").List(
		context.Background(),
		2,
		"from:alerts@example.com",
		"",
		"inbox",
	)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(items) != 1 || items[0]["conversationId"] != "conversation-1" {
		t.Fatalf("unexpected messages: %#v", items)
	}
	if next != serverURL+"/mail/next?state=next" {
		t.Fatalf("unexpected next link: %q", next)
	}
	if !reflect.DeepEqual(requestedScopes, []string{"Mail.Read"}) {
		t.Fatalf("expected Mail.Read scope, got %#v", requestedScopes)
	}

	selected := commaSet(messageListSelect)
	for _, required := range []string{
		"id",
		"parentFolderId",
		"conversationId",
		"subject",
		"from",
		"sender",
		"toRecipients",
		"ccRecipients",
		"receivedDateTime",
		"sentDateTime",
		"isRead",
		"isDraft",
		"hasAttachments",
		"importance",
		"flag",
		"categories",
		"inferenceClassification",
		"webLink",
	} {
		if !selected[required] {
			t.Errorf("message $select is missing %s", required)
		}
	}
	for _, excluded := range []string{"body", "bodyPreview", "attachments"} {
		if selected[excluded] {
			t.Errorf("message $select must not request %s", excluded)
		}
	}
}

func TestListResumedPageIsOpaqueAndDoesNotReapplyMax(t *testing.T) {
	t.Parallel()

	requests := 0
	serverURL := graphTestBaseURL
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.URL.Path != "/mail/next" {
			t.Fatalf("expected /mail/next path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("state"); got != "abc" {
			t.Fatalf("expected opaque state abc, got %q", got)
		}
		if got := r.URL.Query().Get("$skiptoken"); got != "opaque" {
			t.Fatalf("expected opaque skip token, got %q", got)
		}
		if got := r.URL.Query().Get("$top"); got != "" {
			t.Fatalf("resumed request must not add $top, got %q", got)
		}
		if got := r.URL.Query().Get("$select"); got != "" {
			t.Fatalf("resumed request must not add $select, got %q", got)
		}
		if r.Header.Get("ConsistencyLevel") != "eventual" {
			t.Fatalf("expected ConsistencyLevel eventual header, got %q", r.Header.Get("ConsistencyLevel"))
		}
		assertImmutableIDPreference(t, r)
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/mail/next?state=next"}`, serverURL)
	})

	client := newMailTestClient(
		func(context.Context, []string) (string, error) { return "token", nil },
		handler,
	)

	page := serverURL + "/mail/next?%24search=%22foo%22&state=abc&%24skiptoken=opaque"
	items, next, err := New(client, "").List(context.Background(), 1, "", page, "ignored-folder")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one request, got %d", requests)
	}
	if len(items) != 2 {
		t.Fatalf("resumed Graph page must not be trimmed by --max; got %d items", len(items))
	}
	if next != serverURL+"/mail/next?state=next" {
		t.Fatalf("unexpected next link: %s", next)
	}
}

func TestListFoldersInitialPageIncludesDocumentedMetadata(t *testing.T) {
	t.Parallel()

	serverURL := graphTestBaseURL
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/users/person@example.com/mailFolders" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("$top"); got != "25" {
			t.Fatalf("expected $top=25, got %q", got)
		}
		if got := r.URL.Query().Get("$select"); got != mailFolderListSelect {
			t.Fatalf("unexpected $select: %q", got)
		}
		if got := r.URL.Query().Get("includeHiddenFolders"); got != "true" {
			t.Fatalf("expected includeHiddenFolders=true, got %q", got)
		}
		assertImmutableIDPreference(t, r)
		_, _ = fmt.Fprintf(
			w,
			`{"value":[{"id":"inbox-id","displayName":"Inbox","parentFolderId":"root-id","childFolderCount":2,"totalItemCount":10,"unreadItemCount":3}],"@odata.nextLink":"%s/folders/next?state=next"}`,
			serverURL,
		)
	})

	var requestedScopes []string
	client := newMailTestClient(func(_ context.Context, scopes []string) (string, error) {
		requestedScopes = append([]string(nil), scopes...)
		return "token", nil
	}, handler)

	items, next, err := New(client, "person@example.com").ListFolders(context.Background(), 25, "", true)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one folder, got %#v", items)
	}
	for _, key := range []string{
		"id",
		"displayName",
		"parentFolderId",
		"childFolderCount",
		"totalItemCount",
		"unreadItemCount",
	} {
		if _, ok := items[0][key]; !ok {
			t.Errorf("folder response is missing %s: %#v", key, items[0])
		}
	}
	if next != serverURL+"/folders/next?state=next" {
		t.Fatalf("unexpected next link: %q", next)
	}
	if !reflect.DeepEqual(requestedScopes, []string{"Mail.Read"}) {
		t.Fatalf("expected Mail.Read scope, got %#v", requestedScopes)
	}
}

func TestListFoldersResumedPageIsOpaqueAndDoesNotReapplyOptions(t *testing.T) {
	t.Parallel()

	serverURL := graphTestBaseURL
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/folders/next" {
			t.Fatalf("expected /folders/next path, got %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("state"); got != "opaque" {
			t.Fatalf("expected opaque state, got %q", got)
		}
		if got := r.URL.Query().Get("$top"); got != "" {
			t.Fatalf("resumed request must not add $top, got %q", got)
		}
		if got := r.URL.Query().Get("includeHiddenFolders"); got != "" {
			t.Fatalf("resumed request must not add includeHiddenFolders, got %q", got)
		}
		assertImmutableIDPreference(t, r)
		_, _ = fmt.Fprintf(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"%s/folders/next?state=next"}`, serverURL)
	})

	client := newMailTestClient(
		func(context.Context, []string) (string, error) { return "token", nil },
		handler,
	)

	page := serverURL + "/folders/next?state=opaque"
	items, next, err := New(client, "").ListFolders(context.Background(), 1, page, true)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("resumed Graph page must not be trimmed by --max; got %d items", len(items))
	}
	if next != serverURL+"/folders/next?state=next" {
		t.Fatalf("unexpected next link: %q", next)
	}
}

func TestInitialMailPagesPreserveGraphOverdelivery(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"value":[{"id":"1"},{"id":"2"}],"@odata.nextLink":"https://graph.microsoft.com/mail/next?state=after-server-page"}`)
	})

	client := newMailTestClient(
		func(context.Context, []string) (string, error) { return "token", nil },
		handler,
	)
	svc := New(client, "")

	messages, messageNext, err := svc.List(context.Background(), 1, "", "", "")
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(messages) != 2 {
		t.Fatalf("initial message page must preserve all Graph items, got %d", len(messages))
	}
	if messageNext != graphTestBaseURL+"/mail/next?state=after-server-page" {
		t.Fatalf("unexpected message next link: %q", messageNext)
	}

	folders, folderNext, err := svc.ListFolders(context.Background(), 1, "", false)
	if err != nil {
		t.Fatalf("ListFolders failed: %v", err)
	}
	if len(folders) != 2 {
		t.Fatalf("initial folder page must preserve all Graph items, got %d", len(folders))
	}
	if folderNext != graphTestBaseURL+"/mail/next?state=after-server-page" {
		t.Fatalf("unexpected folder next link: %q", folderNext)
	}
}

func TestEndpointsRouteByAuthMode(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name             string
		user             string
		mailboxPath      string
		wantSendMailPath string
	}{
		{
			name:             "delegated uses me endpoints",
			user:             "",
			mailboxPath:      "/me",
			wantSendMailPath: "/me/sendMail",
		},
		{
			name:             "app-only uses user endpoints",
			user:             "person@example.com",
			mailboxPath:      "/users/" + url.PathEscape("person@example.com"),
			wantSendMailPath: "/users/" + url.PathEscape("person@example.com") + "/sendMail",
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					assertImmutableIDPreference(t, r)
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == tc.mailboxPath+"/messages":
					_, _ = fmt.Fprint(w, `{"value":[]}`)
				case r.Method == http.MethodGet && r.URL.Path == tc.mailboxPath+"/mailFolders/Archive/messages":
					_, _ = fmt.Fprint(w, `{"value":[]}`)
				case r.Method == http.MethodGet && r.URL.Path == tc.mailboxPath+"/mailFolders":
					_, _ = fmt.Fprint(w, `{"value":[]}`)
				case r.Method == http.MethodGet && r.URL.Path == tc.mailboxPath+"/messages/message-id":
					_, _ = fmt.Fprint(w, `{"id":"message-id"}`)
				case r.Method == http.MethodPost && r.URL.Path == tc.wantSendMailPath:
					if got := r.Header.Get("Prefer"); got != "" {
						t.Fatalf("send must not request immutable read IDs, got Prefer %q", got)
					}
					w.WriteHeader(http.StatusAccepted)
				default:
					t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
				}
			})

			client := newMailTestClient(
				func(context.Context, []string) (string, error) { return "token", nil },
				handler,
			)

			svc := New(client, tc.user)
			if _, _, err := svc.List(context.Background(), 10, "", "", ""); err != nil {
				t.Fatalf("mailbox-wide List failed: %v", err)
			}
			if _, _, err := svc.List(context.Background(), 10, "", "", "Archive"); err != nil {
				t.Fatalf("folder List failed: %v", err)
			}
			if _, _, err := svc.ListFolders(context.Background(), 10, "", false); err != nil {
				t.Fatalf("ListFolders failed: %v", err)
			}
			if _, err := svc.Get(context.Background(), "message-id"); err != nil {
				t.Fatalf("Get failed: %v", err)
			}
			if err := svc.Send(context.Background(), []string{"to@example.com"}, "subject", "body"); err != nil {
				t.Fatalf("Send failed: %v", err)
			}
		})
	}
}

func TestMoveUsesDocumentedRequestAndReturnsCreatedMessage(t *testing.T) {
	t.Parallel()

	const (
		messageID    = "AAMk/with space+="
		destination  = "folder / +"
		movedMessage = `{"id":"new-message-id","parentFolderId":"folder-id","isRead":false}`
	)

	var requestedScopes []string
	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if got := r.URL.EscapedPath(); got != "/me/messages/AAMk%2Fwith%20space+=/move" {
			t.Fatalf("message ID was not escaped as one path segment: %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("expected application/json, got %q", got)
		}
		assertImmutableIDPreference(t, r)

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode move body: %v", err)
		}
		if !reflect.DeepEqual(payload, map[string]any{"destinationId": destination}) {
			t.Fatalf("unexpected move payload: %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, movedMessage)
	})

	client := newMailTestClient(func(_ context.Context, scopes []string) (string, error) {
		requestedScopes = append([]string(nil), scopes...)
		return "token", nil
	}, handler)

	moved, err := New(client, "").Move(context.Background(), messageID, "  "+destination+"  ")
	if err != nil {
		t.Fatalf("Move failed: %v", err)
	}
	if requests != 1 {
		t.Fatalf("expected one move request, got %d", requests)
	}
	if !reflect.DeepEqual(requestedScopes, []string{"Mail.ReadWrite"}) {
		t.Fatalf("expected Mail.ReadWrite scope, got %#v", requestedScopes)
	}
	if moved["id"] != "new-message-id" || moved["parentFolderId"] != "folder-id" {
		t.Fatalf("unexpected moved message: %#v", moved)
	}
}

func TestArchiveMovesToDocumentedArchiveWellKnownFolder(t *testing.T) {
	t.Parallel()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/users/person@example.com/messages/message-id/move" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode archive body: %v", err)
		}
		if !reflect.DeepEqual(payload, map[string]any{"destinationId": "archive"}) {
			t.Fatalf("archive must use the documented well-known folder: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = fmt.Fprint(w, `{"id":"archived-message-id","parentFolderId":"archive-id"}`)
	})

	client := newMailTestClient(
		func(context.Context, []string) (string, error) { return "token", nil },
		handler,
	)

	archived, err := New(client, "person@example.com").Archive(context.Background(), "message-id")
	if err != nil {
		t.Fatalf("Archive failed: %v", err)
	}
	if archived["id"] != "archived-message-id" {
		t.Fatalf("unexpected archived message: %#v", archived)
	}
}

func TestMarkReadUsesDocumentedPatchAndReturnsUpdatedMessage(t *testing.T) {
	t.Parallel()

	const messageID = "AAMk/read state"
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("expected PATCH, got %s", r.Method)
		}
		if got := r.URL.EscapedPath(); got != "/me/messages/AAMk%2Fread%20state" {
			t.Fatalf("message ID was not escaped as one path segment: %q", got)
		}
		assertImmutableIDPreference(t, r)

		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode update body: %v", err)
		}
		if !reflect.DeepEqual(payload, map[string]any{"isRead": true}) {
			t.Fatalf("unexpected update payload: %#v", payload)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"id":"AAMk/read state","isRead":true}`)
	})

	var requestedScopes []string
	client := newMailTestClient(func(_ context.Context, scopes []string) (string, error) {
		requestedScopes = append([]string(nil), scopes...)
		return "token", nil
	}, handler)

	updated, err := New(client, "").MarkRead(context.Background(), messageID)
	if err != nil {
		t.Fatalf("MarkRead failed: %v", err)
	}
	if updated["isRead"] != true {
		t.Fatalf("expected updated read state, got %#v", updated)
	}
	if !reflect.DeepEqual(requestedScopes, []string{"Mail.ReadWrite"}) {
		t.Fatalf("expected Mail.ReadWrite scope, got %#v", requestedScopes)
	}
}

func TestMailMutationsRequireDocumentedSuccessStatusAndJSONBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		status    int
		body      string
		run       func(*Service) error
		wantErr   string
		confirmed bool
	}{
		{
			name:    "move rejects 200",
			status:  http.StatusOK,
			body:    `{"id":"moved"}`,
			run:     func(s *Service) error { _, err := s.Move(context.Background(), "message-id", "archive"); return err },
			wantErr: "HTTP 201",
		},
		{
			name:      "move rejects malformed 201 body as confirmed response error",
			status:    http.StatusCreated,
			body:      `{not-json`,
			run:       func(s *Service) error { _, err := s.Move(context.Background(), "message-id", "archive"); return err },
			wantErr:   "response",
			confirmed: true,
		},
		{
			name:    "mark-read rejects 204",
			status:  http.StatusNoContent,
			run:     func(s *Service) error { _, err := s.MarkRead(context.Background(), "message-id"); return err },
			wantErr: "HTTP 200",
		},
		{
			name:    "mark-read rejects malformed 200 body",
			status:  http.StatusOK,
			body:    `{not-json`,
			run:     func(s *Service) error { _, err := s.MarkRead(context.Background(), "message-id"); return err },
			wantErr: "decode",
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = fmt.Fprint(w, tc.body)
			})
			client := newMailTestClient(
				func(context.Context, []string) (string, error) { return "token", nil },
				handler,
			)

			err := tc.run(New(client, ""))
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("expected error containing %q, got %v", tc.wantErr, err)
			}
			var confirmedErr *ConfirmedMoveResponseError
			if tc.confirmed != errors.As(err, &confirmedErr) {
				t.Fatalf("confirmed response classification=%v, error=%T %v", tc.confirmed, err, err)
			}
		})
	}
}

func TestMailMutationsRejectBlankInputsBeforeNetworkAccess(t *testing.T) {
	t.Parallel()

	requests := 0
	tokens := 0
	handler := http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests++
	})
	client := newMailTestClient(func(context.Context, []string) (string, error) {
		tokens++
		return "token", nil
	}, handler)
	svc := New(client, "")

	tests := []struct {
		name string
		run  func() error
	}{
		{name: "move message id", run: func() error { _, err := svc.Move(context.Background(), " \t", "archive"); return err }},
		{name: "move destination", run: func() error { _, err := svc.Move(context.Background(), "message-id", "\n"); return err }},
		{name: "archive message id", run: func() error { _, err := svc.Archive(context.Background(), ""); return err }},
		{name: "mark-read message id", run: func() error { _, err := svc.MarkRead(context.Background(), " "); return err }},
	}

	for _, tc := range tests {
		if err := tc.run(); err == nil {
			t.Errorf("%s: expected validation error", tc.name)
		}
	}
	if requests != 0 || tokens != 0 {
		t.Fatalf("validation reached Graph: requests=%d token acquisitions=%d", requests, tokens)
	}
}

func TestMoveDoesNotRetryIndeterminateServerError(t *testing.T) {
	t.Parallel()

	requests := 0
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprint(w, `{"error":{"code":"ServiceUnavailable","message":"try later"}}`)
	})
	client := newMailTestClient(
		func(context.Context, []string) (string, error) { return "token", nil },
		handler,
	)

	_, err := New(client, "").Move(context.Background(), "message-id", "archive")
	if err == nil {
		t.Fatal("expected move error")
	}
	var indeterminateErr *IndeterminateMoveError
	if !errors.As(err, &indeterminateErr) {
		t.Fatalf("expected indeterminate move error, got %T: %v", err, err)
	}
	if requests != 1 {
		t.Fatalf("non-idempotent move must not retry: requests=%d", requests)
	}
}

func TestMoveDoesNotRetryIndeterminateTransportError(t *testing.T) {
	t.Parallel()

	requests := 0
	client := graph.NewClient(func(context.Context, []string) (string, error) {
		return "token", nil
	})
	client.BaseURL = graphTestBaseURL
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return nil, errors.New("connection reset after request write")
	})}

	_, err := New(client, "").Move(context.Background(), "message-id", "archive")
	if err == nil {
		t.Fatal("expected move transport error")
	}
	var indeterminateErr *IndeterminateMoveError
	if !errors.As(err, &indeterminateErr) {
		t.Fatalf("expected indeterminate move error, got %T: %v", err, err)
	}
	if requests != 1 {
		t.Fatalf("non-idempotent transport failure must not retry: requests=%d", requests)
	}
}

func TestMoveTreatsUnreadable4xxResponseAsDeterminateRejection(t *testing.T) {
	t.Parallel()

	requests := 0
	client := graph.NewClient(func(context.Context, []string) (string, error) {
		return "token", nil
	})
	client.BaseURL = graphTestBaseURL
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &errorReadCloser{err: errors.New("connection reset while reading rejection body")},
		}, nil
	})}

	_, err := New(client, "").Move(context.Background(), "message-id", "archive")
	if err == nil {
		t.Fatal("expected move rejection")
	}
	var indeterminateErr *IndeterminateMoveError
	if errors.As(err, &indeterminateErr) {
		t.Fatalf("known HTTP 400 rejection must be determinate, got %T: %v", err, err)
	}
	if !strings.Contains(err.Error(), "HTTP 400") {
		t.Fatalf("expected rejection status in error, got %v", err)
	}
	if requests != 1 {
		t.Fatalf("rejected move must not retry: requests=%d", requests)
	}
}

func TestMoveReportsConfirmedStatusWhenResponseBodyReadFails(t *testing.T) {
	t.Parallel()

	requests := 0
	client := graph.NewClient(func(context.Context, []string) (string, error) {
		return "token", nil
	})
	client.BaseURL = graphTestBaseURL
	client.HTTPClient = &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests++
		return &http.Response{
			StatusCode: http.StatusCreated,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       &errorReadCloser{err: errors.New("connection reset while reading response")},
		}, nil
	})}

	_, err := New(client, "").Move(context.Background(), "message-id", "archive")
	if err == nil {
		t.Fatal("expected response-read error")
	}
	var confirmedErr *ConfirmedMoveResponseError
	if !errors.As(err, &confirmedErr) {
		t.Fatalf("expected confirmed move response error, got %T: %v", err, err)
	}
	if requests != 1 {
		t.Fatalf("move response read failure must not retry: requests=%d", requests)
	}
}

type handlerRoundTripper struct {
	handler http.Handler
}

func (t handlerRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	recorder := httptest.NewRecorder()
	t.handler.ServeHTTP(recorder, req)
	return recorder.Result(), nil
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

type errorReadCloser struct {
	err error
}

func (r *errorReadCloser) Read([]byte) (int, error) {
	return 0, r.err
}

func (r *errorReadCloser) Close() error {
	return nil
}

func newMailTestClient(tokenProvider graph.TokenProvider, handler http.Handler) *graph.Client {
	client := graph.NewClient(tokenProvider)
	client.BaseURL = graphTestBaseURL
	client.HTTPClient = &http.Client{Transport: handlerRoundTripper{handler: handler}}
	return client
}

func assertImmutableIDPreference(t *testing.T, r *http.Request) {
	t.Helper()
	const want = `IdType="ImmutableId"`
	if got := r.Header.Get("Prefer"); got != want {
		t.Fatalf("expected Prefer %q, got %q", want, got)
	}
}

func commaSet(value string) map[string]bool {
	out := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		out[item] = true
	}
	return out
}
