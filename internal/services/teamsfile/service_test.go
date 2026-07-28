package teamsfile

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/graph"
	"github.com/jaredpalmer/mogcli/internal/services/onedrive"
	"github.com/jaredpalmer/mogcli/internal/services/teams"
)

func TestPrepareValidatesHashesAndBuildsStableDryRun(t *testing.T) {
	t.Parallel()

	localPath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}

	prepared, err := Prepare(Request{
		ChatID:    "chat-id",
		LocalPath: localPath,
		Body:      "Please review <today>",
	})
	if err != nil {
		t.Fatalf("Prepare failed: %v", err)
	}
	sum := sha256.Sum256([]byte("hello"))
	if prepared.DisplayName != "report.txt" || prepared.Bytes != 5 || prepared.SHA256 != hex.EncodeToString(sum[:]) {
		t.Fatalf("unexpected prepared request: %#v", prepared)
	}

	plan := prepared.DryRun()
	if !plan.DryRun || plan.Action != "teams.chat-file-send" || plan.ChatID != "chat-id" {
		t.Fatalf("unexpected dry-run identity: %#v", plan)
	}
	if plan.ConflictPolicy != "fail" {
		t.Fatalf("unexpected conflict policy: %q", plan.ConflictPolicy)
	}
	if plan.PermissionPolicy.Role != "read" ||
		!plan.PermissionPolicy.RequireSignIn ||
		plan.PermissionPolicy.SendInvitation ||
		plan.PermissionPolicy.RetainInheritedPermissions {
		t.Fatalf("unexpected permission policy: %#v", plan.PermissionPolicy)
	}
	if len(plan.Stages) != len(operationStages) || plan.RollbackPolicy == "" {
		t.Fatalf("unexpected stages/rollback policy: %#v", plan)
	}
}

func TestPrepareRejectsUnsafeLocalInputs(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	regular := filepath.Join(tempDir, "regular.txt")
	if err := os.WriteFile(regular, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	symlink := filepath.Join(tempDir, "link.txt")
	if err := os.Symlink(regular, symlink); err != nil {
		t.Fatal(err)
	}
	tooLarge := filepath.Join(tempDir, "large.bin")
	largeFile, err := os.Create(tooLarge)
	if err != nil {
		t.Fatal(err)
	}
	if err := largeFile.Truncate(MaxSimpleUploadBytes + 1); err != nil {
		_ = largeFile.Close()
		t.Fatal(err)
	}
	if err := largeFile.Close(); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		request Request
		want    string
	}{
		{
			name:    "symlink",
			request: Request{ChatID: "chat-id", LocalPath: symlink},
			want:    "symlink",
		},
		{
			name:    "directory",
			request: Request{ChatID: "chat-id", LocalPath: tempDir},
			want:    "regular file",
		},
		{
			name:    "simple upload maximum",
			request: Request{ChatID: "chat-id", LocalPath: tooLarge},
			want:    "simple-upload limit",
		},
		{
			name:    "path traversal display name",
			request: Request{ChatID: "chat-id", LocalPath: regular, DisplayName: "../escape.txt"},
			want:    "path separators",
		},
		{
			name:    "control display name",
			request: Request{ChatID: "chat-id", LocalPath: regular, DisplayName: "bad\nname.txt"},
			want:    "controls",
		},
		{
			name:    "unicode formatting control display name",
			request: Request{ChatID: "chat-id", LocalPath: regular, DisplayName: "report\u202Efdp.txt"},
			want:    "formatting controls",
		},
		{
			name:    "reserved display name",
			request: Request{ChatID: "chat-id", LocalPath: regular, DisplayName: "CON.txt"},
			want:    "reserved",
		},
		{
			name:    "body control",
			request: Request{ChatID: "chat-id", LocalPath: regular, Body: "bad\x00body"},
			want:    "control",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := Prepare(test.request)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(test.want)) {
				t.Fatalf("expected error containing %q, got %v", test.want, err)
			}
		})
	}
}

func TestPrepareErrorsDoNotExposeFullParentPath(t *testing.T) {
	t.Parallel()

	parent := filepath.Join(t.TempDir(), "private-parent")
	localPath := filepath.Join(parent, "missing.txt")
	_, err := Prepare(Request{ChatID: "chat-id", LocalPath: localPath})
	if err == nil {
		t.Fatal("expected error")
	}
	if strings.Contains(err.Error(), parent) {
		t.Fatalf("error exposed full local parent path: %v", err)
	}
	if !strings.Contains(err.Error(), "missing.txt") {
		t.Fatalf("error should identify only the basename: %v", err)
	}
}

func TestVerifyOneOnOneRecipientRequiresExactInternalMembership(t *testing.T) {
	t.Parallel()

	current := teams.CurrentUser{ID: "self-id", UserType: "Member"}
	chat := teams.Chat{ID: "chat-id", ChatType: "oneOnOne"}
	validMembers := []teams.ChatMember{
		{
			ODataType:        "#microsoft.graph.aadUserConversationMember",
			UserID:           "self-id",
			TenantID:         "tenant-id",
			UserIdentityType: "aadUser",
			Roles:            []string{"owner"},
		},
		{
			ODataType:        "#microsoft.graph.aadUserConversationMember",
			UserID:           "recipient-id",
			TenantID:         "tenant-id",
			UserIdentityType: "aadUser",
			Roles:            []string{"owner"},
			DisplayName:      "Recipient",
			Email:            "recipient@contoso.example",
		},
	}

	recipient, err := VerifyOneOnOneRecipient(current, chat, validMembers, "")
	if err != nil {
		t.Fatalf("valid membership rejected: %v", err)
	}
	if recipient.UserID != "recipient-id" || recipient.DisplayName != "Recipient" {
		t.Fatalf("unexpected recipient: %#v", recipient)
	}
	basicMembers := cloneMembers(validMembers)
	basicMembers[0].Roles = nil
	basicMembers[1].Roles = nil
	if _, err := VerifyOneOnOneRecipient(current, chat, basicMembers, ""); err != nil {
		t.Fatalf("same-tenant AAD basic members with empty roles should be accepted: %v", err)
	}

	tests := []struct {
		name    string
		current teams.CurrentUser
		chat    teams.Chat
		members []teams.ChatMember
		next    string
	}{
		{
			name:    "signed in guest",
			current: teams.CurrentUser{ID: "self-id", UserType: "Guest"},
			chat:    chat,
			members: cloneMembers(validMembers),
		},
		{
			name:    "group chat",
			current: current,
			chat:    teams.Chat{ID: "chat-id", ChatType: "group"},
			members: cloneMembers(validMembers),
		},
		{
			name:    "additional member page",
			current: current,
			chat:    chat,
			members: cloneMembers(validMembers),
			next:    "https://graph.example/next",
		},
		{
			name:    "missing member",
			current: current,
			chat:    chat,
			members: cloneMembers(validMembers[:1]),
		},
		{
			name:    "non aad member type",
			current: current,
			chat:    chat,
			members: mutateMember(validMembers, 1, func(member *teams.ChatMember) { member.ODataType = "#microsoft.graph.conversationMember" }),
		},
		{
			name:    "missing immutable id",
			current: current,
			chat:    chat,
			members: mutateMember(validMembers, 1, func(member *teams.ChatMember) { member.UserID = "" }),
		},
		{
			name:    "guest role",
			current: current,
			chat:    chat,
			members: mutateMember(validMembers, 1, func(member *teams.ChatMember) { member.Roles = []string{"guest"} }),
		},
		{
			name:    "external tenant",
			current: current,
			chat:    chat,
			members: mutateMember(validMembers, 1, func(member *teams.ChatMember) { member.TenantID = "other-tenant" }),
		},
		{
			name:    "external identity type",
			current: current,
			chat:    chat,
			members: mutateMember(validMembers, 1, func(member *teams.ChatMember) { member.UserIdentityType = "federatedUser" }),
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, err := VerifyOneOnOneRecipient(test.current, test.chat, test.members, test.next); err == nil {
				t.Fatal("expected membership verification to fail")
			}
		})
	}
}

func TestSendSuccessUsesVerifiedHashPermissionAndSingleAttachment(t *testing.T) {
	t.Parallel()

	prepared := preparedTestRequest(t)
	transport := &workflowTransport{content: []byte("hello")}
	service := newWorkflowService(transport)

	result, err := service.Send(context.Background(), prepared)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	if result.MessageID != "message-id" ||
		result.DriveItemID != "item-id" ||
		result.DriveItemSize != 5 ||
		result.PermissionID != "permission-id" ||
		result.RecipientUserID != "recipient-id" ||
		result.DriveItemPath != "approot:/mog-chat-file-fixed" {
		t.Fatalf("unexpected result: %#v", result)
	}
	if result.Bytes != 5 || result.SHA256 != prepared.SHA256 {
		t.Fatalf("unexpected digest metadata: %#v", result)
	}
	if len(transport.deletePaths()) != 0 {
		t.Fatalf("successful send must preserve backing item: %#v", transport.deletePaths())
	}
	if transport.count(http.MethodPost, "/chats/chat-id/messages") != 1 {
		t.Fatalf("expected exactly one message POST, requests=%#v", transport.requestPaths())
	}
	if got := transport.uploadConflictPolicy(); got != "fail" {
		t.Fatalf("expected upload conflict=fail, got %q", got)
	}

	payload := transport.messagePayload()
	body, ok := payload["body"].(map[string]any)
	if !ok || body["contentType"] != "html" {
		t.Fatalf("unexpected message body: %#v", payload["body"])
	}
	wantHTML := `Review &lt;this&gt;<br><attachment id="item-id"></attachment>`
	if body["content"] != wantHTML {
		t.Fatalf("unexpected HTML marker: got %#v want %q", body["content"], wantHTML)
	}
	attachments, ok := payload["attachments"].([]any)
	if !ok || len(attachments) != 1 {
		t.Fatalf("expected exactly one attachment: %#v", payload["attachments"])
	}
	attachment, ok := attachments[0].(map[string]any)
	if !ok ||
		attachment["id"] != "item-id" ||
		attachment["contentType"] != "reference" ||
		attachment["contentUrl"] != "https://contoso.example/item-id" ||
		attachment["name"] != "report.txt" {
		t.Fatalf("unexpected attachment payload: %#v", attachments[0])
	}
}

func TestSendRejectsTamperedPreparedContentBeforeTokenOrGraph(t *testing.T) {
	t.Parallel()

	prepared := preparedTestRequest(t)
	prepared.SHA256 = strings.Repeat("0", 64)
	tokenCalls := 0
	client := graph.NewClient(func(context.Context, []string) (string, error) {
		tokenCalls++
		return "token", nil
	})
	client.HTTPClient = &http.Client{Transport: &workflowTransport{content: []byte("hello")}}

	_, err := New(client).Send(context.Background(), prepared)
	if err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("expected prepared digest validation failure, got %v", err)
	}
	if tokenCalls != 0 {
		t.Fatalf("prepared validation must happen before token acquisition, got %d calls", tokenCalls)
	}
}

func TestSendRecoversAmbiguousOrUnusableUploadResponseByUniquePath(t *testing.T) {
	t.Parallel()

	for _, failAt := range []string{"upload_response", "upload_transport"} {
		failAt := failAt
		t.Run(failAt, func(t *testing.T) {
			t.Parallel()
			transport := &workflowTransport{content: []byte("hello"), failAt: failAt}
			result, err := newWorkflowService(transport).Send(context.Background(), preparedTestRequest(t))
			if err != nil {
				t.Fatalf("expected safe upload recovery, got %v", err)
			}
			if result.MessageID != "message-id" || result.DriveItemID != "item-id" {
				t.Fatalf("unexpected recovered result: %#v", result)
			}
			if transport.count(http.MethodGet, "/me/drive/items/app-root-id:/mog-chat-file-fixed") != 1 {
				t.Fatalf("expected one unique-path recovery lookup: %#v", transport.requestPaths())
			}
		})
	}
}

func TestSendAmbiguousUploadLookupFailureDeletesUniquePath(t *testing.T) {
	t.Parallel()

	transport := &workflowTransport{content: []byte("hello"), failAt: "upload_recovery_lookup"}
	_, err := newWorkflowService(transport).Send(context.Background(), preparedTestRequest(t))
	if err == nil {
		t.Fatal("expected ambiguous upload recovery failure")
	}
	path := "/me/drive/items/app-root-id:/mog-chat-file-fixed"
	if transport.count(http.MethodDelete, path) != 1 {
		t.Fatalf("expected one delete-by-unique-path recovery attempt: %#v", transport.requestPaths())
	}
	if transport.count(http.MethodPost, "/chats/chat-id/messages") != 0 {
		t.Fatalf("message must not be sent after ambiguous upload recovery failure: %#v", transport.requestPaths())
	}
}

func TestSendRollsBackEveryOwnedStageBeforeConfirmedMessage(t *testing.T) {
	t.Parallel()

	tests := []struct {
		failAt         string
		wantPermission bool
	}{
		{failAt: "upload_metadata"},
		{failAt: "readback_request"},
		{failAt: "hash"},
		{failAt: "invite_request"},
		{failAt: "invite_transport"},
		{failAt: "invite_response", wantPermission: true},
		{failAt: "permissions_request", wantPermission: true},
		{failAt: "broad_link", wantPermission: true},
		{failAt: "message_4xx", wantPermission: true},
		{failAt: "message_429", wantPermission: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.failAt, func(t *testing.T) {
			t.Parallel()
			transport := &workflowTransport{content: []byte("hello"), failAt: test.failAt}
			_, err := newWorkflowService(transport).Send(context.Background(), preparedTestRequest(t))
			if err == nil {
				t.Fatal("expected failure")
			}

			deletes := transport.deletePaths()
			if !containsString(deletes, "/me/drive/items/item-id") {
				t.Fatalf("expected operation-created item rollback, deletes=%#v err=%v", deletes, err)
			}
			permissionPath := "/me/drive/items/item-id/permissions/permission-id"
			if containsString(deletes, permissionPath) != test.wantPermission {
				t.Fatalf("permission rollback mismatch: want=%v deletes=%#v err=%v", test.wantPermission, deletes, err)
			}
			if strings.HasPrefix(test.failAt, "invite_") && transport.count(http.MethodPost, "/me/drive/items/item-id/invite") != 1 {
				t.Fatalf("invite POST must not retry: requests=%#v", transport.requestPaths())
			}
			if strings.HasPrefix(test.failAt, "message_") && transport.count(http.MethodPost, "/chats/chat-id/messages") != 1 {
				t.Fatalf("message POST must not retry: requests=%#v", transport.requestPaths())
			}
		})
	}
}

func TestVerifyRecipientOnlyPermissionsRejectsUnexpectedAccess(t *testing.T) {
	t.Parallel()

	base := []onedrive.Permission{
		{ID: "owner-id", Roles: []string{"owner"}, GrantedToV2: &onedrive.IdentitySet{User: &onedrive.Identity{ID: "self-id"}, SiteUser: &onedrive.Identity{ID: "site-self-id"}}},
		{ID: "permission-id", Roles: []string{"read"}, GrantedToV2: &onedrive.IdentitySet{User: &onedrive.Identity{ID: "recipient-id"}, SiteUser: &onedrive.Identity{ID: "site-recipient-id"}}},
	}
	if err := verifyRecipientOnlyPermissions(base, "self-id", "recipient-id", "permission-id"); err != nil {
		t.Fatalf("expected owner plus exact recipient grant to pass: %v", err)
	}
	tests := []struct {
		name       string
		permission onedrive.Permission
	}{
		{name: "anonymous link", permission: onedrive.Permission{ID: "link-id", Roles: []string{"read"}, Link: &onedrive.SharingLink{Scope: "anonymous", Type: "view"}}},
		{name: "organization link", permission: onedrive.Permission{ID: "link-id", Roles: []string{"read"}, Link: &onedrive.SharingLink{Scope: "organization", Type: "view"}}},
		{name: "specific people link", permission: onedrive.Permission{ID: "link-id", Roles: []string{"read"}, Link: &onedrive.SharingLink{Scope: "users", Type: "view"}}},
		{name: "unknown link scope", permission: onedrive.Permission{ID: "link-id", Roles: []string{"read"}, Link: &onedrive.SharingLink{Type: "view"}}},
		{name: "unexpected user", permission: onedrive.Permission{ID: "other-id", Roles: []string{"read"}, GrantedToV2: &onedrive.IdentitySet{User: &onedrive.Identity{ID: "other-user"}}}},
		{name: "group grant", permission: onedrive.Permission{ID: "group-id", Roles: []string{"read"}, GrantedToV2: &onedrive.IdentitySet{Group: &onedrive.Identity{ID: "group-id"}}}},
		{name: "inherited grant", permission: onedrive.Permission{ID: "other-id", Roles: []string{"read"}, GrantedToV2: &onedrive.IdentitySet{User: &onedrive.Identity{ID: "other-user"}}, InheritedFrom: &onedrive.DriveItem{ID: "parent-id"}}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			permissions := append(append([]onedrive.Permission(nil), base...), test.permission)
			if err := verifyRecipientOnlyPermissions(permissions, "self-id", "recipient-id", "permission-id"); err == nil {
				t.Fatalf("expected unexpected access to be rejected: %#v", test.permission)
			}
		})
	}
}

func TestIndeterminateFinalSendPreservesBackingAndDoesNotRetry(t *testing.T) {
	t.Parallel()

	for _, failAt := range []string{"message_transport", "message_request", "message_5xx_response_read"} {
		failAt := failAt
		t.Run(failAt, func(t *testing.T) {
			t.Parallel()

			transport := &workflowTransport{content: []byte("hello"), failAt: failAt}
			_, err := newWorkflowService(transport).Send(context.Background(), preparedTestRequest(t))
			var indeterminate *IndeterminateSendError
			if !errors.As(err, &indeterminate) {
				t.Fatalf("expected IndeterminateSendError, got %T (%v)", err, err)
			}
			if indeterminate.Result.DriveItemID != "item-id" || indeterminate.Result.PermissionID != "permission-id" {
				t.Fatalf("indeterminate error must retain recovery identifiers: %#v", indeterminate.Result)
			}
			if len(transport.deletePaths()) != 0 {
				t.Fatalf("indeterminate final send must preserve backing resources: %#v", transport.deletePaths())
			}
			if transport.count(http.MethodPost, "/chats/chat-id/messages") != 1 {
				t.Fatalf("indeterminate final send must not retry: requests=%#v", transport.requestPaths())
			}
		})
	}
}

func TestConfirmedMessageWithUnusableResponsePreservesBacking(t *testing.T) {
	t.Parallel()

	transport := &workflowTransport{content: []byte("hello"), failAt: "message_response"}
	_, err := newWorkflowService(transport).Send(context.Background(), preparedTestRequest(t))
	var confirmed *ConfirmedSendResponseError
	if !errors.As(err, &confirmed) {
		t.Fatalf("expected ConfirmedSendResponseError, got %T (%v)", err, err)
	}
	if len(transport.deletePaths()) != 0 {
		t.Fatalf("confirmed HTTP 201 must preserve backing resources: %#v", transport.deletePaths())
	}
	if transport.count(http.MethodPost, "/chats/chat-id/messages") != 1 {
		t.Fatalf("confirmed final send must not retry: requests=%#v", transport.requestPaths())
	}
}

func preparedTestRequest(t *testing.T) PreparedRequest {
	t.Helper()
	localPath := filepath.Join(t.TempDir(), "report.txt")
	if err := os.WriteFile(localPath, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	prepared, err := Prepare(Request{
		ChatID:    "chat-id",
		LocalPath: localPath,
		Body:      "Review <this>",
	})
	if err != nil {
		t.Fatal(err)
	}
	return prepared
}

func newWorkflowService(transport *workflowTransport) *Service {
	client := graph.NewClient(func(context.Context, []string) (string, error) {
		return "token", nil
	})
	client.BaseURL = "https://graph.test"
	client.HTTPClient = &http.Client{Transport: transport}
	client.MaxRetries429 = 0
	client.MaxRetries5xx = 0
	client.Breaker = nil

	service := New(client)
	service.newRemoteName = func() (string, error) {
		return "mog-chat-file-fixed", nil
	}
	return service
}

type recordedRequest struct {
	method  string
	path    string
	query   string
	payload map[string]any
}

type workflowTransport struct {
	mu      sync.Mutex
	content []byte
	failAt  string
	records []recordedRequest
}

func (t *workflowTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	var payload map[string]any
	if request.Body != nil && request.Header.Get("Content-Type") == "application/json" {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			return nil, err
		}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &payload); err != nil {
				return nil, err
			}
		}
	}
	t.mu.Lock()
	t.records = append(t.records, recordedRequest{
		method:  request.Method,
		path:    request.URL.Path,
		query:   request.URL.RawQuery,
		payload: payload,
	})
	t.mu.Unlock()

	switch {
	case request.Method == http.MethodGet && request.URL.Path == "/me":
		return graphResponse(http.StatusOK, `{"id":"self-id","displayName":"Self","userType":"Member"}`), nil
	case request.Method == http.MethodGet && request.URL.Path == "/chats/chat-id":
		return graphResponse(http.StatusOK, `{"id":"chat-id","chatType":"oneOnOne"}`), nil
	case request.Method == http.MethodGet && request.URL.Path == "/chats/chat-id/members":
		return graphResponse(http.StatusOK, `{"value":[{"@odata.type":"#microsoft.graph.aadUserConversationMember","userId":"self-id","tenantId":"tenant-id","userIdentityType":"aadUser","roles":["owner"]},{"@odata.type":"#microsoft.graph.aadUserConversationMember","userId":"recipient-id","tenantId":"tenant-id","userIdentityType":"aadUser","roles":["owner"],"displayName":"Recipient","email":"recipient@contoso.example"}]}`), nil
	case request.Method == http.MethodGet && request.URL.Path == "/me/drive/special/approot":
		return graphResponse(http.StatusOK, `{"id":"app-root-id","name":"Mog"}`), nil
	case request.Method == http.MethodPut && request.URL.Path == "/me/drive/items/app-root-id:/mog-chat-file-fixed:/content":
		if t.failAt == "upload_transport" || t.failAt == "upload_recovery_lookup" {
			return nil, errors.New("connection reset after upload write")
		}
		if t.failAt == "upload_response" {
			return graphResponse(http.StatusCreated, `{not-json`), nil
		}
		size := 5
		if t.failAt == "upload_metadata" {
			size = 4
		}
		return graphResponse(http.StatusCreated, fmt.Sprintf(`{"id":"item-id","name":"mog-chat-file-fixed","size":%d,"webUrl":"https://contoso.example/item-id"}`, size)), nil
	case request.Method == http.MethodGet && request.URL.Path == "/me/drive/items/app-root-id:/mog-chat-file-fixed":
		if t.failAt == "upload_recovery_lookup" {
			return graphResponse(http.StatusNotFound, `{"error":{"code":"not_found","message":"missing"}}`), nil
		}
		return graphResponse(http.StatusOK, `{"id":"item-id","name":"mog-chat-file-fixed","size":5,"webUrl":"https://contoso.example/item-id"}`), nil
	case request.Method == http.MethodGet && request.URL.Path == "/me/drive/items/item-id/content":
		if t.failAt == "readback_request" {
			return graphResponse(http.StatusInternalServerError, `{"error":{"code":"read_failed","message":"failed"}}`), nil
		}
		if t.failAt == "hash" {
			return graphResponse(http.StatusOK, "HELLO"), nil
		}
		return graphResponse(http.StatusOK, string(t.content)), nil
	case request.Method == http.MethodPost && request.URL.Path == "/me/drive/items/item-id/invite":
		if t.failAt == "invite_transport" {
			return nil, errors.New("connection reset after permission grant")
		}
		if t.failAt == "invite_request" {
			return graphResponse(http.StatusServiceUnavailable, `{"error":{"code":"busy","message":"failed"}}`), nil
		}
		recipient := "recipient-id"
		if t.failAt == "invite_response" {
			recipient = "wrong-user"
		}
		return graphResponse(http.StatusOK, fmt.Sprintf(`{"value":[{"id":"permission-id","roles":["read"],"grantedToV2":{"user":{"id":%q}}}]}`, recipient)), nil
	case request.Method == http.MethodGet && request.URL.Path == "/me/drive/items/item-id/permissions":
		if t.failAt == "permissions_request" {
			return graphResponse(http.StatusInternalServerError, `{"error":{"code":"permission_failed","message":"failed"}}`), nil
		}
		extra := ""
		if t.failAt == "broad_link" {
			extra = `,{"id":"broad-link","roles":["read"],"link":{"scope":"organization","type":"view"}}`
		}
		return graphResponse(http.StatusOK, `{"value":[{"id":"owner-id","roles":["owner"],"grantedToV2":{"user":{"id":"self-id"}}},{"id":"permission-id","roles":["read"],"grantedToV2":{"user":{"id":"recipient-id"}}}`+extra+`]}`), nil
	case request.Method == http.MethodPost && request.URL.Path == "/chats/chat-id/messages":
		if t.failAt == "message_transport" {
			return nil, errors.New("connection reset after request write")
		}
		if t.failAt == "message_4xx" {
			return graphResponse(http.StatusBadRequest, `{"error":{"code":"bad_request","message":"rejected"}}`), nil
		}
		if t.failAt == "message_429" {
			return graphResponse(http.StatusTooManyRequests, `{"error":{"code":"throttled","message":"rejected"}}`), nil
		}
		if t.failAt == "message_request" {
			return graphResponse(http.StatusServiceUnavailable, `{"error":{"code":"busy","message":"failed"}}`), nil
		}
		if t.failAt == "message_5xx_response_read" {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Status:     "503 Service Unavailable",
				Header:     make(http.Header),
				Body:       failingReadCloser{},
			}, nil
		}
		if t.failAt == "message_response" {
			return graphResponse(http.StatusCreated, `{not-json`), nil
		}
		return graphResponse(http.StatusCreated, `{"id":"message-id"}`), nil
	case request.Method == http.MethodDelete &&
		(request.URL.Path == "/me/drive/items/item-id" ||
			request.URL.Path == "/me/drive/items/item-id/permissions/permission-id" ||
			request.URL.Path == "/me/drive/items/app-root-id:/mog-chat-file-fixed"):
		return graphResponse(http.StatusNoContent, ""), nil
	default:
		return graphResponse(http.StatusNotFound, `{"error":{"code":"not_found","message":"unexpected test request"}}`), nil
	}
}

func (t *workflowTransport) deletePaths() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []string
	for _, record := range t.records {
		if record.method == http.MethodDelete {
			out = append(out, record.path)
		}
	}
	return out
}

func (t *workflowTransport) requestPaths() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]string, 0, len(t.records))
	for _, record := range t.records {
		out = append(out, record.method+" "+record.path)
	}
	return out
}

func (t *workflowTransport) count(method string, path string) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	var count int
	for _, record := range t.records {
		if record.method == method && record.path == path {
			count++
		}
	}
	return count
}

func (t *workflowTransport) uploadConflictPolicy() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, record := range t.records {
		if record.method == http.MethodPut {
			values, _ := url.ParseQuery(record.query)
			return values.Get("@microsoft.graph.conflictBehavior")
		}
	}
	return ""
}

func (t *workflowTransport) messagePayload() map[string]any {
	t.mu.Lock()
	defer t.mu.Unlock()
	for _, record := range t.records {
		if record.method == http.MethodPost && record.path == "/chats/chat-id/messages" {
			return record.payload
		}
	}
	return nil
}

type failingReadCloser struct{}

func (failingReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("response body read failed")
}

func (failingReadCloser) Close() error {
	return nil
}

func graphResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func cloneMembers(value []teams.ChatMember) []teams.ChatMember {
	out := append([]teams.ChatMember(nil), value...)
	for index := range out {
		out[index].Roles = append([]string(nil), out[index].Roles...)
	}
	return out
}

func mutateMember(value []teams.ChatMember, index int, mutate func(*teams.ChatMember)) []teams.ChatMember {
	out := cloneMembers(value)
	mutate(&out[index])
	return out
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
