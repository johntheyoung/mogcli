package secrets

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jaredpalmer/mogcli/internal/config"
)

func TestSetGetDeleteSecretFileBackend(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("MOG_KEYRING_BACKEND", "file")

	key := "mog:test:key"
	value := []byte("hello-world")

	if err := SetSecret(key, value); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	got, err := GetSecret(key)
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if string(got) != string(value) {
		t.Fatalf("unexpected secret value: %q", string(got))
	}

	if err := DeleteSecret(key); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}

	if _, err := GetSecret(key); err == nil {
		t.Fatal("expected error after deleting secret")
	}

	dir, err := os.UserConfigDir()
	if err != nil {
		t.Fatalf("UserConfigDir failed: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "mogcli", "keyring")); statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("stat keyring dir failed: %v", statErr)
	}
}

func TestSetGetDeleteSecretKeychainBackendDispatch(t *testing.T) {
	t.Setenv("MOG_KEYRING_BACKEND", "keychain")
	restore := stubNativeKeychainFuncs(t)

	nativeKeychainAvailableFunc = func() bool { return true }

	ensureCalls := 0
	ensureKeychainAccessFunc = func() error {
		ensureCalls++
		return nil
	}

	var setKey string
	var setValue []byte
	setNativeKeychainSecretFunc = func(key string, value []byte) error {
		setKey = key
		setValue = append([]byte(nil), value...)
		return nil
	}

	getNativeKeychainSecretFunc = func(key string) ([]byte, error) {
		if key != "mog:test:key" {
			t.Fatalf("unexpected get key %q", key)
		}
		return []byte("value-from-keychain"), nil
	}

	deleteKey := ""
	deleteNativeKeychainSecretFunc = func(key string) error {
		deleteKey = key
		return nil
	}

	t.Cleanup(restore)

	if err := SetSecret("mog:test:key", []byte("hello-world")); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}
	if setKey != "mog:test:key" {
		t.Fatalf("unexpected set key: %q", setKey)
	}
	if string(setValue) != "hello-world" {
		t.Fatalf("unexpected set value: %q", string(setValue))
	}

	got, err := GetSecret("mog:test:key")
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if string(got) != "value-from-keychain" {
		t.Fatalf("unexpected get value: %q", string(got))
	}

	if err := DeleteSecret("mog:test:key"); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}
	if deleteKey != "mog:test:key" {
		t.Fatalf("unexpected delete key: %q", deleteKey)
	}

	if ensureCalls != 3 {
		t.Fatalf("expected EnsureKeychainAccess to be called 3 times, got %d", ensureCalls)
	}
}

func TestAutoBackendFallsBackToFileWhenNativeUnavailable(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("MOG_KEYRING_BACKEND", "auto")

	restore := stubNativeKeychainFuncs(t)
	nativeKeychainAvailableFunc = func() bool { return false }
	setNativeKeychainSecretFunc = func(_ string, _ []byte) error {
		t.Fatal("unexpected native keychain set call")
		return nil
	}
	getNativeKeychainSecretFunc = func(_ string) ([]byte, error) {
		t.Fatal("unexpected native keychain get call")
		return nil, nil
	}
	deleteNativeKeychainSecretFunc = func(_ string) error {
		t.Fatal("unexpected native keychain delete call")
		return nil
	}
	t.Cleanup(restore)

	key := "mog:test:auto-file"
	value := []byte("auto-file-value")
	if err := SetSecret(key, value); err != nil {
		t.Fatalf("SetSecret failed: %v", err)
	}

	got, err := GetSecret(key)
	if err != nil {
		t.Fatalf("GetSecret failed: %v", err)
	}
	if string(got) != string(value) {
		t.Fatalf("unexpected secret value: %q", string(got))
	}

	if err := DeleteSecret(key); err != nil {
		t.Fatalf("DeleteSecret failed: %v", err)
	}
}

func TestResolveKeyringBackendInfoPrecedence(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("MOG_KEYRING_BACKEND", "")

	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo default failed: %v", err)
	}
	if info.Value != keyringBackendAuto || info.Source != keyringBackendSourceDefault {
		t.Fatalf("unexpected default backend info: %#v", info)
	}

	if err := config.WriteConfig(config.File{KeyringBackend: "file"}); err != nil {
		t.Fatalf("WriteConfig failed: %v", err)
	}

	info, err = ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo config failed: %v", err)
	}
	if info.Value != keyringBackendFile || info.Source != keyringBackendSourceConfig {
		t.Fatalf("unexpected config backend info: %#v", info)
	}

	t.Setenv("MOG_KEYRING_BACKEND", "keychain")
	info, err = ResolveKeyringBackendInfo()
	if err != nil {
		t.Fatalf("ResolveKeyringBackendInfo env failed: %v", err)
	}
	if info.Value != keyringBackendKeychain || info.Source != keyringBackendSourceEnv {
		t.Fatalf("unexpected env backend info: %#v", info)
	}
}

func TestSetSecretRejectsInvalidBackend(t *testing.T) {
	t.Setenv("MOG_KEYRING_BACKEND", "bogus")
	err := SetSecret("mog:test:key", []byte("value"))
	if err == nil {
		t.Fatal("expected SetSecret to fail for invalid backend")
	}
	if !errors.Is(err, errInvalidKeyringBackend) {
		t.Fatalf("expected errInvalidKeyringBackend, got %v", err)
	}
}

func TestSetSecretRejectsUnavailableNativeKeychain(t *testing.T) {
	t.Setenv("MOG_KEYRING_BACKEND", "keychain")
	restore := stubNativeKeychainFuncs(t)
	nativeKeychainAvailableFunc = func() bool { return false }
	t.Cleanup(restore)

	err := SetSecret("mog:test:key", []byte("value"))
	if err == nil {
		t.Fatal("expected SetSecret to fail for unavailable native keychain")
	}
	if !errors.Is(err, errNativeKeychainUnavailable) {
		t.Fatalf("expected errNativeKeychainUnavailable, got %v", err)
	}
}

func TestKeyringPasswordTreatsEmptyAsIntentional(t *testing.T) {
	t.Setenv("MOG_KEYRING_PASSWORD", "")
	password, ok := keyringPassword()
	if !ok {
		t.Fatal("expected empty keyring password to be treated as set")
	}
	if password != "" {
		t.Fatalf("expected empty password, got %q", password)
	}
}

func stubNativeKeychainFuncs(t *testing.T) func() {
	t.Helper()

	originalAvailable := nativeKeychainAvailableFunc
	originalEnsure := ensureKeychainAccessFunc
	originalSet := setNativeKeychainSecretFunc
	originalGet := getNativeKeychainSecretFunc
	originalDelete := deleteNativeKeychainSecretFunc

	return func() {
		nativeKeychainAvailableFunc = originalAvailable
		ensureKeychainAccessFunc = originalEnsure
		setNativeKeychainSecretFunc = originalSet
		getNativeKeychainSecretFunc = originalGet
		deleteNativeKeychainSecretFunc = originalDelete
	}
}
