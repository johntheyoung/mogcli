package onedrive

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/graph"
)

var listOneDriveScopes = []string{"Files.Read"}
var getOneDriveScopes = []string{"Files.Read"}
var putOneDriveScopes = []string{"Files.ReadWrite"}
var mkdirOneDriveScopes = []string{"Files.ReadWrite"}
var removeOneDriveScopes = []string{"Files.ReadWrite"}
var shareOneDriveScopes = []string{"Files.ReadWrite"}

type DriveItem struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Size            int64           `json:"size"`
	WebURL          string          `json:"webUrl"`
	ParentReference ParentReference `json:"parentReference"`
}

type ParentReference struct {
	DriveID string `json:"driveId"`
	ID      string `json:"id"`
	Path    string `json:"path"`
}

type Identity struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName"`
}

type IdentitySet struct {
	User     *Identity `json:"user,omitempty"`
	SiteUser *Identity `json:"siteUser,omitempty"`
	Group    *Identity `json:"group,omitempty"`
}

type SharingLink struct {
	Scope string `json:"scope"`
	Type  string `json:"type"`
}

type PermissionError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Permission struct {
	ID                    string           `json:"id"`
	Roles                 []string         `json:"roles"`
	Link                  *SharingLink     `json:"link,omitempty"`
	GrantedTo             *IdentitySet     `json:"grantedTo,omitempty"`
	GrantedToV2           *IdentitySet     `json:"grantedToV2,omitempty"`
	GrantedToIdentities   []IdentitySet    `json:"grantedToIdentities,omitempty"`
	GrantedToIdentitiesV2 []IdentitySet    `json:"grantedToIdentitiesV2,omitempty"`
	InheritedFrom         *DriveItem       `json:"inheritedFrom,omitempty"`
	Error                 *PermissionError `json:"error,omitempty"`
}

type Service struct {
	client      *graph.Client
	appOnlyUser string
}

func New(client *graph.Client, appOnlyUser string) *Service {
	return &Service{client: client, appOnlyUser: strings.TrimSpace(appOnlyUser)}
}

func (s *Service) List(ctx context.Context, remotePath string, max int, page string) ([]map[string]any, string, error) {
	if strings.TrimSpace(page) != "" {
		return s.client.Paginate(ctx, strings.TrimSpace(page), nil, listOneDriveScopes, max)
	}

	endpoint := s.driveEndpoint() + "/root/children"
	if cleaned := normalizeRemotePath(remotePath); cleaned != "/" {
		endpoint = s.driveEndpoint() + "/root:" + cleaned + ":/children"
	}

	query := url.Values{}
	if max > 0 {
		query.Set("$top", fmt.Sprintf("%d", max))
	}

	return s.client.Paginate(ctx, endpoint, query, listOneDriveScopes, max)
}

func (s *Service) Get(ctx context.Context, remotePath string) ([]byte, error) {
	endpoint := s.driveEndpoint() + "/root:" + normalizeRemotePath(remotePath) + ":/content"
	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, nil, nil, getOneDriveScopes, nil)
	if err != nil {
		return nil, err
	}

	return body, nil
}

func (s *Service) Stat(ctx context.Context, remotePath string) (map[string]any, error) {
	var payload map[string]any
	err := s.client.DoJSON(ctx, http.MethodGet, s.driveEndpoint()+"/root:"+normalizeRemotePath(remotePath), nil, nil, getOneDriveScopes, &payload)
	return payload, err
}

func (s *Service) Put(ctx context.Context, remotePath string, content []byte) error {
	endpoint := s.driveEndpoint() + "/root:" + normalizeRemotePath(remotePath) + ":/content"
	_, _, err := s.client.Do(ctx, http.MethodPut, endpoint, nil, content, putOneDriveScopes, nil)
	return err
}

// AppRoot resolves the private application folder in the signed-in user's
// OneDrive. Graph creates it on first access.
func (s *Service) AppRoot(ctx context.Context) (DriveItem, error) {
	var item DriveItem
	err := s.client.DoJSON(ctx, http.MethodGet, s.driveEndpoint()+"/special/approot", nil, nil, putOneDriveScopes, &item)
	if err != nil {
		return DriveItem{}, err
	}
	if strings.TrimSpace(item.ID) == "" {
		return DriveItem{}, fmt.Errorf("app root response did not include an item id")
	}
	return item, nil
}

// UploadSimpleFail creates a new item below parentID with Graph simple upload.
// Automatic retries are disabled because a failed transport after creation
// cannot safely distinguish a committed upload from a pre-send failure.
func (s *Service) UploadSimpleFail(ctx context.Context, parentID string, remoteName string, content []byte) (DriveItem, error) {
	parentID = strings.TrimSpace(parentID)
	remoteName = strings.TrimSpace(remoteName)
	if parentID == "" {
		return DriveItem{}, fmt.Errorf("parent item id is required")
	}
	if remoteName == "" || strings.ContainsAny(remoteName, `/\`) {
		return DriveItem{}, fmt.Errorf("remote item name is invalid")
	}

	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(parentID) + ":/" + url.PathEscape(remoteName) + ":/content"
	query := url.Values{}
	query.Set("@microsoft.graph.conflictBehavior", "fail")

	resp, body, err := s.client.DoWithOptions(
		ctx,
		http.MethodPut,
		endpoint,
		query,
		content,
		putOneDriveScopes,
		nil,
		graph.RequestOptions{DisableRetries: true},
	)
	if err != nil {
		if shouldRecoverSimpleUpload(err) {
			return s.recoverAmbiguousSimpleUpload(ctx, parentID, remoteName, err)
		}
		return DriveItem{}, err
	}
	if resp == nil || resp.StatusCode != http.StatusCreated {
		status := 0
		if resp != nil {
			status = resp.StatusCode
		}
		return DriveItem{}, fmt.Errorf("simple upload did not confirm creation (status %d)", status)
	}

	var item DriveItem
	if err := json.Unmarshal(body, &item); err != nil {
		return s.recoverAmbiguousSimpleUpload(ctx, parentID, remoteName, fmt.Errorf("decode uploaded drive item: %w", err))
	}
	if strings.TrimSpace(item.ID) == "" {
		return s.recoverAmbiguousSimpleUpload(ctx, parentID, remoteName, fmt.Errorf("simple upload response did not include an item id"))
	}
	return item, nil
}

func (s *Service) ItemByParentAndName(ctx context.Context, parentID string, remoteName string) (DriveItem, error) {
	parentID = strings.TrimSpace(parentID)
	remoteName = strings.TrimSpace(remoteName)
	if parentID == "" || remoteName == "" || strings.ContainsAny(remoteName, `/\`) {
		return DriveItem{}, fmt.Errorf("parent item id and safe remote item name are required")
	}

	var item DriveItem
	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(parentID) + ":/" + url.PathEscape(remoteName)
	err := s.client.DoJSON(ctx, http.MethodGet, endpoint, nil, nil, putOneDriveScopes, &item)
	if err != nil {
		return DriveItem{}, err
	}
	if strings.TrimSpace(item.ID) == "" {
		return DriveItem{}, fmt.Errorf("drive item lookup did not include an item id")
	}
	return item, nil
}

func (s *Service) ContentByID(ctx context.Context, itemID string) ([]byte, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, fmt.Errorf("drive item id is required")
	}
	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(itemID) + "/content"
	_, body, err := s.client.Do(ctx, http.MethodGet, endpoint, nil, nil, putOneDriveScopes, nil)
	return body, err
}

// InviteRead grants one immutable AAD object ID direct read access. Invite is
// deliberately never retried to avoid duplicate non-idempotent permission
// grants.
func (s *Service) InviteRead(ctx context.Context, itemID string, recipientUserID string) ([]Permission, error) {
	itemID = strings.TrimSpace(itemID)
	recipientUserID = strings.TrimSpace(recipientUserID)
	if itemID == "" {
		return nil, fmt.Errorf("drive item id is required")
	}
	if recipientUserID == "" {
		return nil, fmt.Errorf("recipient user id is required")
	}

	payload := map[string]any{
		"recipients": []map[string]string{
			{"objectId": recipientUserID},
		},
		"roles":                      []string{"read"},
		"requireSignIn":              true,
		"sendInvitation":             false,
		"retainInheritedPermissions": false,
	}
	var result struct {
		Value []Permission `json:"value"`
	}
	err := s.client.DoJSONWithOptions(
		ctx,
		http.MethodPost,
		s.driveEndpoint()+"/items/"+url.PathEscape(itemID)+"/invite",
		nil,
		payload,
		shareOneDriveScopes,
		&result,
		graph.RequestOptions{DisableRetries: true},
	)
	return result.Value, err
}

func (s *Service) Permissions(ctx context.Context, itemID string) ([]Permission, string, error) {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return nil, "", fmt.Errorf("drive item id is required")
	}
	var result struct {
		Value []Permission `json:"value"`
		Next  string       `json:"@odata.nextLink"`
	}
	err := s.client.DoJSON(
		ctx,
		http.MethodGet,
		s.driveEndpoint()+"/items/"+url.PathEscape(itemID)+"/permissions",
		nil,
		nil,
		shareOneDriveScopes,
		&result,
	)
	return result.Value, strings.TrimSpace(result.Next), err
}

func (s *Service) RemovePermission(ctx context.Context, itemID string, permissionID string) error {
	itemID = strings.TrimSpace(itemID)
	permissionID = strings.TrimSpace(permissionID)
	if itemID == "" || permissionID == "" {
		return fmt.Errorf("drive item id and permission id are required")
	}
	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(itemID) + "/permissions/" + url.PathEscape(permissionID)
	_, _, err := s.client.Do(ctx, http.MethodDelete, endpoint, nil, nil, shareOneDriveScopes, nil)
	return err
}

func (s *Service) RemoveByID(ctx context.Context, itemID string) error {
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return fmt.Errorf("drive item id is required")
	}
	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(itemID)
	_, _, err := s.client.Do(ctx, http.MethodDelete, endpoint, nil, nil, removeOneDriveScopes, nil)
	return err
}

func (s *Service) recoverAmbiguousSimpleUpload(ctx context.Context, parentID string, remoteName string, cause error) (DriveItem, error) {
	recovered, recoverErr := s.ItemByParentAndName(ctx, parentID, remoteName)
	if recoverErr == nil {
		return recovered, nil
	}

	endpoint := s.driveEndpoint() + "/items/" + url.PathEscape(parentID) + ":/" + url.PathEscape(remoteName)
	_, _, removeErr := s.client.Do(ctx, http.MethodDelete, endpoint, nil, nil, removeOneDriveScopes, nil)
	if removeErr != nil {
		return DriveItem{}, errors.Join(cause, fmt.Errorf("recover ambiguous upload metadata: %w", recoverErr), fmt.Errorf("remove ambiguous created item by unique path: %w", removeErr))
	}
	return DriveItem{}, errors.Join(cause, fmt.Errorf("recover ambiguous upload metadata: %w", recoverErr))
}

func shouldRecoverSimpleUpload(err error) bool {
	var transportErr *graph.TransportError
	if errors.As(err, &transportErr) {
		return true
	}
	var responseReadErr *graph.ResponseReadError
	if errors.As(err, &responseReadErr) {
		return true
	}
	var apiErr *graph.APIError
	return errors.As(err, &apiErr) && apiErr.Status >= http.StatusInternalServerError
}

func (s *Service) Mkdir(ctx context.Context, remotePath string) error {
	cleaned := normalizeRemotePath(remotePath)
	parent := path.Dir(cleaned)
	if parent == "." {
		parent = "/"
	}
	name := path.Base(cleaned)
	if name == "." || name == "/" || strings.TrimSpace(name) == "" {
		return fmt.Errorf("invalid folder path: %s", remotePath)
	}

	endpoint := s.driveEndpoint() + "/root/children"
	if parent != "/" {
		endpoint = s.driveEndpoint() + "/root:" + parent + ":/children"
	}

	payload := map[string]any{
		"name":                              name,
		"folder":                            map[string]any{},
		"@microsoft.graph.conflictBehavior": "rename",
	}

	_, _, err := s.client.Do(ctx, http.MethodPost, endpoint, nil, payload, mkdirOneDriveScopes, nil)
	return err
}

func (s *Service) Remove(ctx context.Context, remotePath string) error {
	endpoint := s.driveEndpoint() + "/root:" + normalizeRemotePath(remotePath)
	_, _, err := s.client.Do(ctx, http.MethodDelete, endpoint, nil, nil, removeOneDriveScopes, nil)
	return err
}

func (s *Service) driveEndpoint() string {
	if s.appOnlyUser != "" {
		return "/users/" + url.PathEscape(s.appOnlyUser) + "/drive"
	}
	return "/me/drive"
}

func normalizeRemotePath(p string) string {
	trimmed := strings.TrimSpace(p)
	if trimmed == "" || trimmed == "/" {
		return "/"
	}

	trimmed = strings.TrimPrefix(trimmed, "~/")
	trimmed = strings.TrimPrefix(trimmed, "./")
	trimmed = strings.TrimPrefix(trimmed, "/")

	segments := strings.Split(trimmed, "/")
	escaped := make([]string, 0, len(segments))
	for _, segment := range segments {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		escaped = append(escaped, url.PathEscape(segment))
	}

	if len(escaped) == 0 {
		return "/"
	}

	return "/" + filepath.ToSlash(strings.Join(escaped, "/"))
}
