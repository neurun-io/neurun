package netpolicy

import (
	"context"
	"errors"
	"net/netip"
	"sync"
	"testing"
)

type fakeResolver struct {
	addresses map[string][]netip.Addr
	err       error
}

func (resolver *fakeResolver) LookupNetIP(
	_ context.Context,
	_ string,
	host string,
) ([]netip.Addr, error) {
	if resolver.err != nil {
		return nil, resolver.err
	}
	return append([]netip.Addr(nil), resolver.addresses[host]...), nil
}

func TestPolicyValidatesAndNormalizesPublicTarget(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{addresses: map[string][]netip.Addr{
		"api.example.com": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("2606:4700:4700::1111"),
		},
	}}
	policy, err := NewPolicy(Options{
		Resolver:     resolver,
		AllowedHosts: []string{"*.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}

	target, err := policy.ParseAndValidate(context.Background(), "HTTPS://API.EXAMPLE.COM./v1?q=yes")
	if err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	if got, want := target.URL.String(), "https://api.example.com/v1?q=yes"; got != want {
		t.Fatalf("normalized URL = %q, want %q", got, want)
	}
	if target.Host != "api.example.com" || target.Port != 443 {
		t.Fatalf("target authority = %s:%d", target.Host, target.Port)
	}
	if got, want := len(target.Addresses), 2; got != want {
		t.Fatalf("address count = %d, want %d", got, want)
	}

	// Results do not alias resolver-owned storage.
	target.Addresses[0] = netip.MustParseAddr("1.1.1.1")
	second, err := policy.ParseAndValidate(context.Background(), "https://api.example.com/v1")
	if err != nil {
		t.Fatal(err)
	}
	if second.Addresses[0] != netip.MustParseAddr("93.184.216.34") {
		t.Fatal("validated target mutated resolver state")
	}
}

func TestPolicyRejectsMalformedOrUnsafeURLs(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
		"example.com": {netip.MustParseAddr("93.184.216.34")},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		rawURL  string
		wantErr error
	}{
		{name: "missing scheme", rawURL: "example.com/path", wantErr: ErrInvalidURL},
		{name: "unsupported scheme", rawURL: "ftp://example.com/file", wantErr: ErrInvalidURL},
		{name: "credentials", rawURL: "https://user:secret@example.com/", wantErr: ErrInvalidURL},
		{name: "fragment", rawURL: "https://example.com/#private", wantErr: ErrInvalidURL},
		{name: "missing host", rawURL: "https:///path", wantErr: ErrInvalidURL},
		{name: "unicode hostname", rawURL: "https://exämple.com/", wantErr: ErrInvalidURL},
		{name: "invalid port", rawURL: "https://example.com:notaport/", wantErr: ErrInvalidURL},
		{name: "zero port", rawURL: "https://example.com:0/", wantErr: ErrInvalidURL},
		{name: "unsafe service port", rawURL: "https://example.com:22/", wantErr: ErrPortNotAllowed},
		{name: "cross-scheme standard port", rawURL: "http://example.com:443/", wantErr: ErrPortNotAllowed},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			_, err := policy.ParseAndValidate(context.Background(), test.rawURL)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want errors.Is(%v)", err, test.wantErr)
			}
		})
	}
}

func TestPolicyRejectsUnsafeAddressClasses(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{})
	if err != nil {
		t.Fatal(err)
	}
	addresses := []string{
		"0.0.0.0",
		"10.0.0.1",
		"100.64.0.1",
		"127.0.0.1",
		"169.254.169.254",
		"172.16.0.1",
		"192.168.1.1",
		"198.18.0.1",
		"224.0.0.1",
		"255.255.255.255",
		"::",
		"::1",
		"64:ff9b::7f00:1",
		"fc00::1",
		"fe80::1",
		"ff02::1",
		"2002:7f00:1::",
	}
	for _, address := range addresses {
		address := address
		t.Run(address, func(t *testing.T) {
			t.Parallel()
			rawURL := "http://" + address + "/"
			if netip.MustParseAddr(address).Is6() {
				rawURL = "http://[" + address + "]/"
			}
			_, err := policy.ParseAndValidate(context.Background(), rawURL)
			if !errors.Is(err, ErrBlockedAddress) {
				t.Fatalf("error = %v, want ErrBlockedAddress", err)
			}
		})
	}
}

func TestPolicyRejectsEntireMixedDNSAnswer(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
		"mixed.example": {
			netip.MustParseAddr("93.184.216.34"),
			netip.MustParseAddr("127.0.0.1"),
		},
	}}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = policy.ParseAndValidate(context.Background(), "https://mixed.example/")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("error = %v, want ErrBlockedAddress", err)
	}
}

func TestPolicyHostRulesDenyPrecedesAllow(t *testing.T) {
	t.Parallel()

	resolver := &fakeResolver{addresses: map[string][]netip.Addr{
		"api.example.com":     {netip.MustParseAddr("93.184.216.34")},
		"blocked.example.com": {netip.MustParseAddr("93.184.216.34")},
		"example.com":         {netip.MustParseAddr("93.184.216.34")},
		"other.example.net":   {netip.MustParseAddr("93.184.216.34")},
	}}
	policy, err := NewPolicy(Options{
		Resolver:     resolver,
		AllowedHosts: []string{"*.example.com"},
		DeniedHosts:  []string{"blocked.example.com"},
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := policy.ParseAndValidate(context.Background(), "https://api.example.com/"); err != nil {
		t.Fatalf("allowlisted subdomain rejected: %v", err)
	}
	for _, rawURL := range []string{
		"https://blocked.example.com/",
		"https://example.com/",
		"https://other.example.net/",
	} {
		if _, err := policy.ParseAndValidate(context.Background(), rawURL); !errors.Is(err, ErrBlockedHost) {
			t.Fatalf("%s error = %v, want ErrBlockedHost", rawURL, err)
		}
	}
}

func TestPolicyExplicitLocalTestOverrides(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{
		Resolver: &fakeResolver{addresses: map[string][]netip.Addr{
			"localhost": {netip.MustParseAddr("127.0.0.1")},
		}},
		AllowPrivateNetworks:   true,
		AdditionalAllowedPorts: []int{18080},
	})
	if err != nil {
		t.Fatal(err)
	}

	target, err := policy.ParseAndValidate(context.Background(), "http://localhost:18080/test")
	if err != nil {
		t.Fatalf("private test target rejected: %v", err)
	}
	if target.Port != 18080 || target.Addresses[0] != netip.MustParseAddr("127.0.0.1") {
		t.Fatalf("unexpected target: %#v", target)
	}
}

func TestPolicyRejectsResolutionFailuresAndInvalidOptions(t *testing.T) {
	t.Parallel()

	policy, err := NewPolicy(Options{Resolver: &fakeResolver{addresses: map[string][]netip.Addr{}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = policy.ParseAndValidate(context.Background(), "https://missing.example/")
	if !errors.Is(err, ErrResolution) {
		t.Fatalf("empty DNS error = %v, want ErrResolution", err)
	}

	invalid := []Options{
		{AdditionalAllowedPorts: []int{0}},
		{AdditionalAllowedPorts: []int{65536}},
		{AllowedHosts: []string{"*"}},
		{DeniedHosts: []string{"bad*host.example"}},
		{AllowedHosts: []string{""}},
	}
	for _, options := range invalid {
		if _, err := NewPolicy(options); !errors.Is(err, ErrInvalidPolicy) {
			t.Fatalf("NewPolicy(%#v) error = %v, want ErrInvalidPolicy", options, err)
		}
	}
}

type sequenceResolver struct {
	mu        sync.Mutex
	addresses [][]netip.Addr
}

func (resolver *sequenceResolver) LookupNetIP(
	_ context.Context,
	_ string,
	_ string,
) ([]netip.Addr, error) {
	resolver.mu.Lock()
	defer resolver.mu.Unlock()
	if len(resolver.addresses) == 0 {
		return nil, errors.New("unexpected extra lookup")
	}
	result := append([]netip.Addr(nil), resolver.addresses[0]...)
	resolver.addresses = resolver.addresses[1:]
	return result, nil
}
