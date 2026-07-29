package auth

import (
	"reflect"
	"testing"
)

func TestDelegatedScopesForMailIncludesReadWrite(t *testing.T) {
	t.Parallel()

	got, err := DelegatedScopesForWorkloads([]string{"mail"})
	if err != nil {
		t.Fatalf("DelegatedScopesForWorkloads failed: %v", err)
	}
	want := []string{
		"openid",
		"profile",
		"offline_access",
		"User.Read",
		"Mail.Read",
		"Mail.Send",
		"Mail.ReadWrite",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("mail workload scopes = %#v, want %#v", got, want)
	}
}

func TestAllDelegatedWorkloadScopesIncludesMailReadWrite(t *testing.T) {
	t.Parallel()

	for _, scope := range AllDelegatedWorkloadScopes {
		if scope == "Mail.ReadWrite" {
			return
		}
	}
	t.Fatal("AllDelegatedWorkloadScopes must include Mail.ReadWrite")
}
