package secrets

import (
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/jaredpalmer/mogcli/internal/config"
)

const (
	keyringPasswordEnv = "MOG_KEYRING_PASSWORD" //nolint:gosec // env var name, not a credential
	keyringBackendEnv  = "MOG_KEYRING_BACKEND"  //nolint:gosec // env var name, not a credential
)

var (
	errMissingSecretKey          = errors.New("missing secret key")
	errInvalidKeyringBackend     = errors.New("invalid keyring backend")
	errNativeKeychainUnavailable = errors.New("native keychain backend unavailable")

	nativeKeychainAvailableFunc    = nativeKeychainAvailable
	ensureKeychainAccessFunc       = EnsureKeychainAccess
	setNativeKeychainSecretFunc    = setNativeKeychainSecret
	getNativeKeychainSecretFunc    = getNativeKeychainSecret
	deleteNativeKeychainSecretFunc = deleteNativeKeychainSecret
)

type KeyringBackendInfo struct {
	Value  string
	Source string
}

const (
	keyringBackendSourceEnv     = "env"
	keyringBackendSourceConfig  = "config"
	keyringBackendSourceDefault = "default"
	keyringBackendAuto          = "auto"
	keyringBackendKeychain      = "keychain"
	keyringBackendFile          = "file"
)

func ResolveKeyringBackendInfo() (KeyringBackendInfo, error) {
	if v := normalizeKeyringBackend(os.Getenv(keyringBackendEnv)); v != "" {
		return KeyringBackendInfo{Value: v, Source: keyringBackendSourceEnv}, nil
	}

	cfg, err := config.ReadConfig()
	if err != nil {
		return KeyringBackendInfo{}, fmt.Errorf("resolve keyring backend: %w", err)
	}

	if cfg.KeyringBackend != "" {
		if v := normalizeKeyringBackend(cfg.KeyringBackend); v != "" {
			return KeyringBackendInfo{Value: v, Source: keyringBackendSourceConfig}, nil
		}
	}

	return KeyringBackendInfo{Value: keyringBackendAuto, Source: keyringBackendSourceDefault}, nil
}

func normalizeKeyringBackend(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func keyringPassword() (string, bool) {
	// Treat empty strings as intentional (for headless/non-interactive use).
	return os.LookupEnv(keyringPasswordEnv)
}

func SetSecret(key string, value []byte) error {
	normalized, err := normalizeSecretKey(key)
	if err != nil {
		return err
	}

	backend, err := resolveSecretBackend()
	if err != nil {
		return err
	}

	switch backend {
	case keyringBackendKeychain:
		if err := ensureKeychainAccessFunc(); err != nil {
			return fmt.Errorf("access keychain: %w", err)
		}
		if err := setNativeKeychainSecretFunc(normalized, value); err != nil {
			return fmt.Errorf("store secret: %w", err)
		}
		return nil
	case keyringBackendFile:
		return setFileSecret(normalized, value)
	default:
		return fmt.Errorf("%w: %q", errInvalidKeyringBackend, backend)
	}
}

func GetSecret(key string) ([]byte, error) {
	normalized, err := normalizeSecretKey(key)
	if err != nil {
		return nil, err
	}

	backend, err := resolveSecretBackend()
	if err != nil {
		return nil, err
	}

	switch backend {
	case keyringBackendKeychain:
		if err := ensureKeychainAccessFunc(); err != nil {
			return nil, fmt.Errorf("access keychain: %w", err)
		}
		secret, err := getNativeKeychainSecretFunc(normalized)
		if err != nil {
			return nil, fmt.Errorf("read secret: %w", err)
		}
		return secret, nil
	case keyringBackendFile:
		return getFileSecret(normalized)
	default:
		return nil, fmt.Errorf("%w: %q", errInvalidKeyringBackend, backend)
	}
}

func DeleteSecret(key string) error {
	normalized, err := normalizeSecretKey(key)
	if err != nil {
		return err
	}

	backend, err := resolveSecretBackend()
	if err != nil {
		return err
	}

	switch backend {
	case keyringBackendKeychain:
		if err := ensureKeychainAccessFunc(); err != nil {
			return fmt.Errorf("access keychain: %w", err)
		}
		if err := deleteNativeKeychainSecretFunc(normalized); err != nil {
			return fmt.Errorf("delete secret: %w", err)
		}
		return nil
	case keyringBackendFile:
		return deleteFileSecret(normalized)
	default:
		return fmt.Errorf("%w: %q", errInvalidKeyringBackend, backend)
	}
}

func resolveSecretBackend() (string, error) {
	info, err := ResolveKeyringBackendInfo()
	if err != nil {
		return "", err
	}

	switch info.Value {
	case "", keyringBackendAuto:
		if nativeKeychainAvailableFunc() {
			return keyringBackendKeychain, nil
		}
		return keyringBackendFile, nil
	case keyringBackendKeychain:
		if !nativeKeychainAvailableFunc() {
			return "", fmt.Errorf("%w: %q", errNativeKeychainUnavailable, keyringBackendKeychain)
		}
		return keyringBackendKeychain, nil
	case keyringBackendFile:
		return keyringBackendFile, nil
	default:
		return "", fmt.Errorf("%w: %q (expected auto, keychain, or file)", errInvalidKeyringBackend, info.Value)
	}
}

func normalizeSecretKey(key string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return "", errMissingSecretKey
	}
	return key, nil
}

func setFileSecret(key string, value []byte) error {
	path, err := secretPathFromNormalized(key)
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, value, 0o600); err != nil {
		return fmt.Errorf("store secret: %w", err)
	}
	return nil
}

func getFileSecret(key string) ([]byte, error) {
	path, err := secretPathFromNormalized(key)
	if err != nil {
		return nil, err
	}

	item, err := os.ReadFile(path) //nolint:gosec // controlled path from secret key
	if err != nil {
		return nil, fmt.Errorf("read secret: %w", err)
	}
	return item, nil
}

func deleteFileSecret(key string) error {
	path, err := secretPathFromNormalized(key)
	if err != nil {
		return err
	}

	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("delete secret: %w", err)
	}
	return nil
}

func secretPath(key string) (string, error) {
	normalized, err := normalizeSecretKey(key)
	if err != nil {
		return "", err
	}
	return secretPathFromNormalized(normalized)
}

func secretPathFromNormalized(key string) (string, error) {
	dir, err := config.EnsureKeyringDir()
	if err != nil {
		return "", fmt.Errorf("ensure secret dir: %w", err)
	}

	safe := base64.RawURLEncoding.EncodeToString([]byte(key))
	return filepath.Join(dir, safe), nil
}
