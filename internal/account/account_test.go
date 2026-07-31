package account

import "testing"

func TestValidDisplayNameRejectsControlCharactersAndBounds(t *testing.T) {
	t.Parallel()
	if !validDisplayName("Ada Lovelace") {
		t.Fatal("ordinary display name was rejected")
	}
	for _, invalid := range []string{"", "line\nbreak", string(make([]byte, 129))} {
		if validDisplayName(invalid) {
			t.Fatalf("invalid display name accepted: %q", invalid)
		}
	}
}

func TestNormalizeScopesDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()
	scopes := normalizeScopes([]string{"users:read", " deployments:read ", "users:read"})
	if len(scopes) != 2 || scopes[0] != "deployments:read" || scopes[1] != "users:read" {
		t.Fatalf("scopes = %#v", scopes)
	}
}
