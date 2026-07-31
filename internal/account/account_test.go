package account

import (
	"reflect"
	"testing"
)

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

// The pgx stdlib driver returns a text[] column as a string, so the scopes
// queries render it as JSON. Decoding must accept both the string and []byte
// forms a driver may hand database/sql, and must reject anything else rather
// than yielding empty scopes — an unnoticed decode failure once rejected every
// API key.
func TestScopeListScansDriverRepresentations(t *testing.T) {
	t.Parallel()
	for name, src := range map[string]any{
		"string": `["users:read", "deployments:write"]`,
		"bytes":  []byte(`["users:read", "deployments:write"]`),
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			var scopes scopeList
			if err := scopes.Scan(src); err != nil {
				t.Fatalf("scan %s: %v", name, err)
			}
			if want := []string{"users:read", "deployments:write"}; !reflect.DeepEqual([]string(scopes), want) {
				t.Fatalf("scopes = %#v, want %#v", scopes, want)
			}
		})
	}
}

func TestScopeListScanEmptyAndInvalid(t *testing.T) {
	t.Parallel()
	var scopes scopeList
	if err := scopes.Scan(`[]`); err != nil || len(scopes) != 0 {
		t.Fatalf("empty array: scopes = %#v, err = %v", scopes, err)
	}
	if err := scopes.Scan(nil); err != nil || scopes != nil {
		t.Fatalf("nil: scopes = %#v, err = %v", scopes, err)
	}
	// A bare Postgres array literal is exactly what a missing to_jsonb would
	// produce, so it has to fail loudly rather than decode to nothing.
	if err := scopes.Scan(`{users:read}`); err == nil {
		t.Fatal("array literal was accepted as JSON")
	}
	if err := scopes.Scan(42); err == nil {
		t.Fatal("non-textual driver value was accepted")
	}
}

func TestNormalizeScopesDeduplicatesAndSorts(t *testing.T) {
	t.Parallel()
	scopes := normalizeScopes([]string{"users:read", " deployments:read ", "users:read"})
	if len(scopes) != 2 || scopes[0] != "deployments:read" || scopes[1] != "users:read" {
		t.Fatalf("scopes = %#v", scopes)
	}
}
