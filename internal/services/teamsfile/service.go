package teamsfile

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jaredpalmer/mogcli/internal/graph"
	"github.com/jaredpalmer/mogcli/internal/services/onedrive"
	"github.com/jaredpalmer/mogcli/internal/services/teams"
)

const MaxSimpleUploadBytes int64 = 50 * 1024 * 1024
const MaxBodyBytes = 4096

const actionName = "teams.chat-file-send"

var operationStages = []string{
	"validate_local_file",
	"resolve_signed_in_user",
	"verify_one_on_one_chat",
	"resolve_private_mog_app_folder",
	"upload_new_drive_item",
	"verify_uploaded_content",
	"grant_direct_read_permission",
	"verify_recipient_only_permission_set",
	"send_single_reference_attachment_message",
}

const rollbackPolicy = "Before a confirmed message HTTP 201, remove only the operation-created permission and drive item. Preserve both after a confirmed message or an indeterminate final-send transport/server-error outcome."

type Request struct {
	ChatID      string
	LocalPath   string
	DisplayName string
	Body        string
}

type PreparedRequest struct {
	ChatID      string
	DisplayName string
	Body        string
	Bytes       int64
	SHA256      string
	content     []byte
}

type PermissionPolicy struct {
	Type                       string `json:"type"`
	Role                       string `json:"role"`
	Recipient                  string `json:"recipient"`
	RequireSignIn              bool   `json:"require_sign_in"`
	SendInvitation             bool   `json:"send_invitation"`
	RetainInheritedPermissions bool   `json:"retain_inherited_permissions"`
}

type DryRunResult struct {
	DryRun           bool             `json:"dry_run"`
	Action           string           `json:"action"`
	ChatID           string           `json:"chat_id"`
	Filename         string           `json:"filename"`
	Bytes            int64            `json:"bytes"`
	SHA256           string           `json:"sha256"`
	MaxBytes         int64            `json:"max_bytes"`
	ConflictPolicy   string           `json:"conflict_policy"`
	PermissionPolicy PermissionPolicy `json:"permission_policy"`
	Stages           []string         `json:"stages"`
	RollbackPolicy   string           `json:"rollback_policy"`
}

type Recipient struct {
	UserID      string `json:"user_id"`
	DisplayName string `json:"display_name,omitempty"`
	Email       string `json:"email,omitempty"`
}

type Result struct {
	Action           string `json:"action"`
	ChatID           string `json:"chat_id"`
	MessageID        string `json:"message_id,omitempty"`
	Filename         string `json:"filename"`
	Bytes            int64  `json:"bytes"`
	SHA256           string `json:"sha256"`
	DriveItemID      string `json:"drive_item_id"`
	DriveItemPath    string `json:"drive_item_path"`
	DriveItemName    string `json:"drive_item_name"`
	DriveItemWebURL  string `json:"drive_item_web_url"`
	DriveItemSize    int64  `json:"drive_item_size"`
	PermissionID     string `json:"permission_id"`
	RecipientUserID  string `json:"recipient_user_id"`
	RecipientDisplay string `json:"recipient_display_name,omitempty"`
	RecipientEmail   string `json:"recipient_email,omitempty"`
}

type IndeterminateSendError struct {
	Result Result
	Err    error
}

func (e *IndeterminateSendError) Error() string {
	return fmt.Sprintf(
		"Teams message send outcome is indeterminate; backing drive item %s and permission %s were preserved to avoid breaking a possibly delivered message: %v",
		e.Result.DriveItemID,
		e.Result.PermissionID,
		e.Err,
	)
}

func (e *IndeterminateSendError) Unwrap() error {
	return e.Err
}

type ConfirmedSendResponseError struct {
	Result Result
	Err    error
}

func (e *ConfirmedSendResponseError) Error() string {
	return fmt.Sprintf(
		"Teams confirmed message creation with HTTP 201; backing drive item %s and permission %s were preserved, but the message response was unusable: %v",
		e.Result.DriveItemID,
		e.Result.PermissionID,
		e.Err,
	)
}

func (e *ConfirmedSendResponseError) Unwrap() error {
	return e.Err
}

type Service struct {
	teams         *teams.Service
	drive         *onedrive.Service
	newRemoteName func() (string, error)
}

func New(client *graph.Client) *Service {
	return &Service{
		teams:         teams.New(client),
		drive:         onedrive.New(client, ""),
		newRemoteName: randomRemoteName,
	}
}

func Prepare(request Request) (PreparedRequest, error) {
	chatID := strings.TrimSpace(request.ChatID)
	if chatID == "" {
		return PreparedRequest{}, fmt.Errorf("chat id is required")
	}
	if len(chatID) > 2048 || containsUnsafeControl(chatID, false) {
		return PreparedRequest{}, fmt.Errorf("chat id contains unsafe characters or is too long")
	}

	body := strings.TrimSpace(request.Body)
	if !utf8.ValidString(body) {
		return PreparedRequest{}, fmt.Errorf("message body must be valid UTF-8")
	}
	if len(body) > MaxBodyBytes {
		return PreparedRequest{}, fmt.Errorf("message body exceeds the %d-byte limit", MaxBodyBytes)
	}
	if containsUnsafeControl(body, true) {
		return PreparedRequest{}, fmt.Errorf("message body contains unsafe control characters")
	}

	localPath := strings.TrimSpace(request.LocalPath)
	if localPath == "" {
		return PreparedRequest{}, fmt.Errorf("local file path is required")
	}
	defaultName := filepath.Base(filepath.Clean(localPath))
	displayName := request.DisplayName
	if strings.TrimSpace(displayName) == "" {
		displayName = defaultName
	}
	displayName, err := validateDisplayName(displayName)
	if err != nil {
		return PreparedRequest{}, err
	}

	before, err := os.Lstat(localPath)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("inspect local file %q: %v", defaultName, filesystemCause(err))
	}
	if before.Mode()&os.ModeSymlink != 0 {
		return PreparedRequest{}, fmt.Errorf("local file %q must not be a symlink", defaultName)
	}
	if !before.Mode().IsRegular() {
		return PreparedRequest{}, fmt.Errorf("local file %q must be a regular file", defaultName)
	}
	if before.Size() > MaxSimpleUploadBytes {
		return PreparedRequest{}, fmt.Errorf("local file %q exceeds the %d-byte simple-upload limit", defaultName, MaxSimpleUploadBytes)
	}

	file, err := os.Open(localPath)
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("open local file %q: %v", defaultName, filesystemCause(err))
	}
	defer file.Close()

	opened, err := file.Stat()
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("inspect opened local file %q: %v", defaultName, filesystemCause(err))
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return PreparedRequest{}, fmt.Errorf("local file %q changed during validation", defaultName)
	}

	content, err := io.ReadAll(io.LimitReader(file, MaxSimpleUploadBytes+1))
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("read local file %q: %v", defaultName, filesystemCause(err))
	}
	if int64(len(content)) > MaxSimpleUploadBytes {
		return PreparedRequest{}, fmt.Errorf("local file %q exceeds the %d-byte simple-upload limit", defaultName, MaxSimpleUploadBytes)
	}
	after, err := file.Stat()
	if err != nil {
		return PreparedRequest{}, fmt.Errorf("reinspect local file %q: %v", defaultName, filesystemCause(err))
	}
	if !os.SameFile(before, after) || before.Size() != int64(len(content)) || after.Size() != int64(len(content)) {
		return PreparedRequest{}, fmt.Errorf("local file %q changed while it was read", defaultName)
	}

	digest := sha256.Sum256(content)
	return PreparedRequest{
		ChatID:      chatID,
		DisplayName: displayName,
		Body:        body,
		Bytes:       int64(len(content)),
		SHA256:      hex.EncodeToString(digest[:]),
		content:     content,
	}, nil
}

func (p PreparedRequest) DryRun() DryRunResult {
	return DryRunResult{
		DryRun:         true,
		Action:         actionName,
		ChatID:         p.ChatID,
		Filename:       p.DisplayName,
		Bytes:          p.Bytes,
		SHA256:         p.SHA256,
		MaxBytes:       MaxSimpleUploadBytes,
		ConflictPolicy: "fail",
		PermissionPolicy: PermissionPolicy{
			Type:                       "direct_aad_user",
			Role:                       "read",
			Recipient:                  "verified_other_internal_one_on_one_chat_member",
			RequireSignIn:              true,
			SendInvitation:             false,
			RetainInheritedPermissions: false,
		},
		Stages:         append([]string(nil), operationStages...),
		RollbackPolicy: rollbackPolicy,
	}
}

func (s *Service) Send(ctx context.Context, prepared PreparedRequest) (result Result, retErr error) {
	if s == nil || s.teams == nil || s.drive == nil || s.newRemoteName == nil {
		return Result{}, fmt.Errorf("teams file service is not initialized")
	}
	if err := validatePrepared(prepared); err != nil {
		return Result{}, err
	}

	currentUser, err := s.teams.CurrentUser(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("resolve signed-in user: %w", err)
	}
	chat, err := s.teams.Chat(ctx, prepared.ChatID)
	if err != nil {
		return Result{}, fmt.Errorf("resolve Teams chat: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(chat.ID), prepared.ChatID) {
		return Result{}, fmt.Errorf("resolved Teams chat id did not match the supplied chat id")
	}
	members, next, err := s.teams.ChatMembersForVerification(ctx, prepared.ChatID)
	if err != nil {
		return Result{}, fmt.Errorf("list Teams chat members: %w", err)
	}
	recipient, err := VerifyOneOnOneRecipient(currentUser, chat, members, next)
	if err != nil {
		return Result{}, err
	}

	appRoot, err := s.drive.AppRoot(ctx)
	if err != nil {
		return Result{}, fmt.Errorf("resolve private mog app folder: %w", err)
	}
	remoteName, err := s.newRemoteName()
	if err != nil {
		return Result{}, fmt.Errorf("generate unique remote item name: %w", err)
	}

	var itemOwned bool
	var permissionID string
	preserveBacking := false
	defer func() {
		if retErr == nil || preserveBacking || !itemOwned {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 20*time.Second)
		defer cancel()
		if rollbackErr := s.rollback(cleanupCtx, result.DriveItemID, permissionID); rollbackErr != nil {
			retErr = errors.Join(retErr, fmt.Errorf("rollback operation-owned backing resources: %w", rollbackErr))
		}
	}()

	item, err := s.drive.UploadSimpleFail(ctx, appRoot.ID, remoteName, prepared.content)
	if err != nil {
		return Result{}, fmt.Errorf("upload new drive item with conflict policy fail: %w", err)
	}
	itemOwned = true
	result = Result{
		Action:           actionName,
		ChatID:           prepared.ChatID,
		Filename:         prepared.DisplayName,
		Bytes:            prepared.Bytes,
		SHA256:           prepared.SHA256,
		DriveItemID:      strings.TrimSpace(item.ID),
		DriveItemPath:    "approot:/" + remoteName,
		DriveItemName:    strings.TrimSpace(item.Name),
		DriveItemWebURL:  strings.TrimSpace(item.WebURL),
		DriveItemSize:    item.Size,
		RecipientUserID:  recipient.UserID,
		RecipientDisplay: recipient.DisplayName,
		RecipientEmail:   recipient.Email,
	}
	if err := validateUploadedItem(item, remoteName, prepared.Bytes); err != nil {
		return result, err
	}

	downloaded, err := s.drive.ContentByID(ctx, item.ID)
	if err != nil {
		return result, fmt.Errorf("read uploaded drive item back by id: %w", err)
	}
	downloadedDigest := sha256.Sum256(downloaded)
	if int64(len(downloaded)) != prepared.Bytes || hex.EncodeToString(downloadedDigest[:]) != prepared.SHA256 {
		return result, fmt.Errorf("uploaded drive item content verification failed: byte count or SHA-256 mismatch")
	}

	invited, err := s.drive.InviteRead(ctx, item.ID, recipient.UserID)
	if len(invited) == 1 {
		permissionID = strings.TrimSpace(invited[0].ID)
		result.PermissionID = permissionID
	}
	if err != nil {
		return result, fmt.Errorf("grant direct read permission: %w", err)
	}
	if len(invited) != 1 || permissionID == "" {
		return result, fmt.Errorf("direct read invite did not return exactly one identifiable permission")
	}
	if err := verifyDirectPermission(invited[0], recipient.UserID, permissionID); err != nil {
		return result, fmt.Errorf("verify returned direct read permission: %w", err)
	}

	permissions, permissionsNext, err := s.drive.Permissions(ctx, item.ID)
	if err != nil {
		return result, fmt.Errorf("list drive item permissions: %w", err)
	}
	if permissionsNext != "" {
		return result, fmt.Errorf("drive item permission verification was incomplete because Graph returned another page")
	}
	if err := verifyRecipientOnlyPermissions(permissions, currentUser.ID, recipient.UserID, permissionID); err != nil {
		return result, fmt.Errorf("listed permissions did not prove recipient-only access: %w", err)
	}

	message, err := s.teams.SendReferenceAttachment(ctx, prepared.ChatID, prepared.Body, teams.ReferenceAttachment{
		ID:         item.ID,
		ContentURL: item.WebURL,
		Name:       prepared.DisplayName,
	})
	if err != nil {
		var indeterminate *teams.IndeterminateMessageSendError
		if errors.As(err, &indeterminate) {
			preserveBacking = true
			return result, &IndeterminateSendError{Result: result, Err: err}
		}
		var confirmed *teams.ConfirmedMessageResponseError
		if errors.As(err, &confirmed) {
			preserveBacking = true
			return result, &ConfirmedSendResponseError{Result: result, Err: err}
		}
		return result, fmt.Errorf("send Teams reference attachment message: %w", err)
	}

	result.MessageID = strings.TrimSpace(message.ID)
	preserveBacking = true
	return result, nil
}

func VerifyOneOnOneRecipient(current teams.CurrentUser, chat teams.Chat, members []teams.ChatMember, next string) (Recipient, error) {
	return verifyOneOnOneRecipient(current, chat, members, next)
}

func verifyOneOnOneRecipient(current teams.CurrentUser, chat teams.Chat, members []teams.ChatMember, next string) (Recipient, error) {
	currentID := strings.TrimSpace(current.ID)
	if currentID == "" {
		return Recipient{}, fmt.Errorf("signed-in user response did not include an immutable id")
	}
	if !strings.EqualFold(strings.TrimSpace(current.UserType), "Member") {
		return Recipient{}, fmt.Errorf("signed-in user must be an internal directory member")
	}
	if strings.TrimSpace(chat.ID) == "" || !strings.EqualFold(strings.TrimSpace(chat.ChatType), "oneOnOne") {
		return Recipient{}, fmt.Errorf("supplied chat is not a verified one-on-one chat")
	}
	if strings.TrimSpace(next) != "" {
		return Recipient{}, fmt.Errorf("chat member verification was incomplete because Graph returned another page")
	}
	if len(members) != 2 {
		return Recipient{}, fmt.Errorf("one-on-one chat must contain exactly two members; got %d", len(members))
	}

	var self *teams.ChatMember
	var other *teams.ChatMember
	seen := map[string]struct{}{}
	for index := range members {
		member := &members[index]
		if strings.TrimSpace(member.ODataType) != "#microsoft.graph.aadUserConversationMember" {
			return Recipient{}, fmt.Errorf("chat member is not an AAD user conversation member")
		}
		memberID := strings.TrimSpace(member.UserID)
		if memberID == "" {
			return Recipient{}, fmt.Errorf("chat member is missing immutable userId")
		}
		key := strings.ToLower(memberID)
		if _, ok := seen[key]; ok {
			return Recipient{}, fmt.Errorf("chat members contain a duplicate immutable userId")
		}
		seen[key] = struct{}{}
		if strings.TrimSpace(member.TenantID) == "" {
			return Recipient{}, fmt.Errorf("chat member is missing tenantId needed to prove internal membership")
		}
		if hasRole(member.Roles, "guest") {
			return Recipient{}, fmt.Errorf("chat member has a guest role")
		}
		for _, role := range member.Roles {
			if strings.TrimSpace(role) != "" && !strings.EqualFold(strings.TrimSpace(role), "owner") {
				return Recipient{}, fmt.Errorf("chat member has an ambiguous role %q", role)
			}
		}
		identityType := strings.TrimSpace(member.UserIdentityType)
		if identityType != "" && !strings.EqualFold(identityType, "aadUser") && !strings.EqualFold(identityType, "onPremisesAadUser") {
			return Recipient{}, fmt.Errorf("chat member has external or ambiguous identity type %q", identityType)
		}

		if strings.EqualFold(memberID, currentID) {
			self = member
		} else {
			other = member
		}
	}
	if self == nil || other == nil {
		return Recipient{}, fmt.Errorf("chat members did not contain exactly the signed-in user and one other AAD member")
	}
	if !strings.EqualFold(strings.TrimSpace(self.TenantID), strings.TrimSpace(other.TenantID)) {
		return Recipient{}, fmt.Errorf("other chat member is external to the signed-in user's tenant")
	}

	return Recipient{
		UserID:      strings.TrimSpace(other.UserID),
		DisplayName: safeMetadata(other.DisplayName, 256),
		Email:       safeMetadata(other.Email, 320),
	}, nil
}

func (s *Service) rollback(ctx context.Context, itemID string, permissionID string) error {
	var rollbackErr error
	if strings.TrimSpace(permissionID) != "" {
		if err := s.drive.RemovePermission(ctx, itemID, permissionID); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove created permission: %w", err))
		}
	}
	if strings.TrimSpace(itemID) != "" {
		if err := s.drive.RemoveByID(ctx, itemID); err != nil {
			rollbackErr = errors.Join(rollbackErr, fmt.Errorf("remove created drive item: %w", err))
		}
	}
	return rollbackErr
}

func validateUploadedItem(item onedrive.DriveItem, remoteName string, expectedBytes int64) error {
	if strings.TrimSpace(item.ID) == "" {
		return fmt.Errorf("uploaded drive item response did not include an id")
	}
	if strings.TrimSpace(item.Name) != remoteName {
		return fmt.Errorf("uploaded drive item name did not match the unique requested name")
	}
	if item.Size != expectedBytes {
		return fmt.Errorf("uploaded drive item size did not match the local byte count")
	}
	webURL, err := url.Parse(strings.TrimSpace(item.WebURL))
	if err != nil || !strings.EqualFold(webURL.Scheme, "https") || strings.TrimSpace(webURL.Host) == "" || webURL.User != nil {
		return fmt.Errorf("uploaded drive item response did not include a safe HTTPS webUrl")
	}
	return nil
}

func verifyDirectPermission(permission onedrive.Permission, recipientUserID string, permissionID string) error {
	if permission.Error != nil {
		return fmt.Errorf("permission invite returned an item-level error %q", strings.TrimSpace(permission.Error.Code))
	}
	if strings.TrimSpace(permission.ID) == "" || !strings.EqualFold(strings.TrimSpace(permission.ID), strings.TrimSpace(permissionID)) {
		return fmt.Errorf("permission id did not match the invite result")
	}
	if permission.Link != nil {
		return fmt.Errorf("direct permission unexpectedly contained a sharing link")
	}
	if permission.InheritedFrom != nil {
		return fmt.Errorf("permission was inherited instead of directly granted")
	}
	if len(permission.Roles) != 1 || !strings.EqualFold(strings.TrimSpace(permission.Roles[0]), "read") {
		return fmt.Errorf("permission role was not exactly read")
	}

	userIDs, err := permissionUserIDs(permission)
	if err != nil {
		return err
	}
	if len(userIDs) != 1 || !strings.EqualFold(userIDs[0], strings.TrimSpace(recipientUserID)) {
		return fmt.Errorf("permission did not identify only the immutable recipient userId")
	}
	return nil
}

func validatePrepared(prepared PreparedRequest) error {
	if strings.TrimSpace(prepared.ChatID) == "" ||
		len(prepared.ChatID) > 2048 ||
		containsUnsafeControl(prepared.ChatID, false) {
		return fmt.Errorf("prepared chat id is invalid")
	}
	displayName, err := validateDisplayName(prepared.DisplayName)
	if err != nil || displayName != prepared.DisplayName {
		return fmt.Errorf("prepared displayed filename is invalid")
	}
	if !utf8.ValidString(prepared.Body) ||
		len(prepared.Body) > MaxBodyBytes ||
		containsUnsafeControl(prepared.Body, true) {
		return fmt.Errorf("prepared message body is invalid")
	}
	if prepared.Bytes < 0 ||
		prepared.Bytes > MaxSimpleUploadBytes ||
		len(prepared.content) != int(prepared.Bytes) {
		return fmt.Errorf("prepared file content is invalid")
	}
	digest := sha256.Sum256(prepared.content)
	if !strings.EqualFold(prepared.SHA256, hex.EncodeToString(digest[:])) {
		return fmt.Errorf("prepared file SHA-256 is invalid")
	}
	return nil
}

func verifyRecipientOnlyPermissions(permissions []onedrive.Permission, ownerUserID string, recipientUserID string, permissionID string) error {
	ownerUserID = strings.TrimSpace(ownerUserID)
	recipientUserID = strings.TrimSpace(recipientUserID)
	permissionID = strings.TrimSpace(permissionID)
	if ownerUserID == "" || recipientUserID == "" || permissionID == "" {
		return fmt.Errorf("owner, recipient, and permission ids are required")
	}

	var recipientMatches int
	var ownerMatches int
	for _, permission := range permissions {
		if permission.Error != nil {
			return fmt.Errorf("permission list contained an item-level error %q", strings.TrimSpace(permission.Error.Code))
		}
		if strings.TrimSpace(permission.ID) == "" {
			return fmt.Errorf("permission list contained an item without an id")
		}
		if permission.Link != nil {
			scope := strings.TrimSpace(permission.Link.Scope)
			if scope == "" {
				scope = "unknown"
			}
			return fmt.Errorf("drive item has an unexpected %s-scope sharing link", scope)
		}
		if permission.InheritedFrom != nil {
			return fmt.Errorf("drive item retained an inherited permission")
		}

		if strings.EqualFold(strings.TrimSpace(permission.ID), permissionID) {
			if err := verifyDirectPermission(permission, recipientUserID, permissionID); err != nil {
				return err
			}
			recipientMatches++
			continue
		}

		userIDs, err := permissionUserIDs(permission)
		if err != nil {
			return err
		}
		if len(userIDs) != 1 || !strings.EqualFold(userIDs[0], ownerUserID) {
			return fmt.Errorf("drive item has a permission for an unexpected user or group")
		}
		if len(permission.Roles) != 1 || !strings.EqualFold(strings.TrimSpace(permission.Roles[0]), "owner") {
			return fmt.Errorf("signed-in user's retained permission was not exactly owner")
		}
		ownerMatches++
	}
	if recipientMatches != 1 {
		return fmt.Errorf("expected exactly one direct recipient read permission; got %d", recipientMatches)
	}
	if ownerMatches > 1 {
		return fmt.Errorf("expected at most one explicit signed-in owner permission; got %d", ownerMatches)
	}
	return nil
}

func permissionUserIDs(permission onedrive.Permission) ([]string, error) {
	identities := make([]onedrive.IdentitySet, 0, 2+len(permission.GrantedToIdentities)+len(permission.GrantedToIdentitiesV2))
	if permission.GrantedTo != nil {
		identities = append(identities, *permission.GrantedTo)
	}
	if permission.GrantedToV2 != nil {
		identities = append(identities, *permission.GrantedToV2)
	}
	identities = append(identities, permission.GrantedToIdentities...)
	identities = append(identities, permission.GrantedToIdentitiesV2...)
	if len(identities) == 0 {
		return nil, fmt.Errorf("permission did not identify a user")
	}

	users := map[string]string{}
	for _, identitySet := range identities {
		if identitySet.Group != nil {
			return nil, fmt.Errorf("permission identified a group")
		}
		if identitySet.User == nil || strings.TrimSpace(identitySet.User.ID) == "" {
			return nil, fmt.Errorf("permission contained an empty or unsupported identity")
		}
		userID := strings.TrimSpace(identitySet.User.ID)
		users[strings.ToLower(userID)] = userID
	}
	if len(users) != 1 {
		return nil, fmt.Errorf("permission identified multiple users")
	}
	for _, userID := range users {
		return []string{userID}, nil
	}
	return nil, fmt.Errorf("permission did not identify a user")
}

func validateDisplayName(value string) (string, error) {
	name := strings.TrimSpace(value)
	if name == "" || name == "." || name == ".." {
		return "", fmt.Errorf("displayed filename is empty or unsafe")
	}
	if !utf8.ValidString(name) || len([]rune(name)) > 255 {
		return "", fmt.Errorf("displayed filename must be valid UTF-8 and at most 255 characters")
	}
	if strings.ContainsAny(name, `<>:"/\|?*`) || containsUnsafeControl(name, false) || containsUnicodeFormatControl(name) {
		return "", fmt.Errorf("displayed filename contains path separators, reserved characters, controls, or formatting controls")
	}
	if strings.HasSuffix(name, ".") || strings.HasSuffix(name, " ") {
		return "", fmt.Errorf("displayed filename must not end with a dot or space")
	}
	base := strings.TrimSuffix(name, filepath.Ext(name))
	switch strings.ToUpper(base) {
	case "CON", "PRN", "AUX", "NUL",
		"COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return "", fmt.Errorf("displayed filename is reserved")
	}
	return name, nil
}

func containsUnsafeControl(value string, allowWhitespace bool) bool {
	for _, r := range value {
		if !unicode.IsControl(r) {
			continue
		}
		if allowWhitespace && (r == '\n' || r == '\r' || r == '\t') {
			continue
		}
		return true
	}
	return false
}

func containsUnicodeFormatControl(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Cf, r) {
			return true
		}
	}
	return false
}

func hasRole(roles []string, target string) bool {
	for _, role := range roles {
		if strings.EqualFold(strings.TrimSpace(role), target) {
			return true
		}
	}
	return false
}

func safeMetadata(value string, max int) string {
	value = strings.TrimSpace(value)
	if value == "" || !utf8.ValidString(value) || len([]rune(value)) > max || containsUnsafeControl(value, false) {
		return ""
	}
	return value
}

func filesystemCause(err error) error {
	var pathErr *os.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil {
		return pathErr.Err
	}
	return err
}

func randomRemoteName() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return "mog-chat-file-" + hex.EncodeToString(value[:]), nil
}
