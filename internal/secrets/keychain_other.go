//go:build !darwin

package secrets

import "errors"

var errNativeKeychainUnsupported = errors.New("native keychain backend unsupported on this platform")

func nativeKeychainAvailable() bool { return false }

func setNativeKeychainSecret(_ string, _ []byte) error {
	return errNativeKeychainUnsupported
}

func getNativeKeychainSecret(_ string) ([]byte, error) {
	return nil, errNativeKeychainUnsupported
}

func deleteNativeKeychainSecret(_ string) error {
	return errNativeKeychainUnsupported
}

func IsKeychainLockedError(_ string) bool { return false }
func CheckKeychainLocked() bool           { return false }
func UnlockKeychain() error               { return nil }
func EnsureKeychainAccess() error         { return nil }
