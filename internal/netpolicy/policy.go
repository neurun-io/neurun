// Package netpolicy validates and safely dials outbound HTTP targets. It is
// intended to be one layer in a defense-in-depth egress policy; production
// deployments should also enforce network policy at the host or container
// boundary.
package netpolicy

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
)

var (
	ErrInvalidURL     = errors.New("invalid outbound URL")
	ErrBlockedHost    = errors.New("outbound host is blocked")
	ErrBlockedAddress = errors.New("outbound address is blocked")
	ErrPortNotAllowed = errors.New("outbound port is not allowed")
	ErrResolution     = errors.New("outbound host resolution failed")
	ErrInvalidPolicy  = errors.New("invalid outbound policy")
)

// Resolver is the subset of net.Resolver used by Policy. net.DefaultResolver
// satisfies this interface, and tests can provide a deterministic resolver.
type Resolver interface {
	LookupNetIP(ctx context.Context, network, host string) ([]netip.Addr, error)
}

// Options configures an immutable Policy.
type Options struct {
	Resolver Resolver

	// AllowPrivateNetworks is an explicit escape hatch for deterministic local
	// tests and trusted private deployments. It permits loopback, private,
	// link-local, carrier-grade NAT, and other special-use unicast addresses.
	// Multicast and unspecified addresses remain blocked.
	AllowPrivateNetworks bool

	// AllowedHosts is an optional allowlist. An empty list permits any host not
	// matched by DeniedHosts. Rules are exact hostnames or "*.example.com".
	AllowedHosts []string

	// DeniedHosts always takes precedence over AllowedHosts. Rules are exact
	// hostnames, "*.example.com", or "*".
	DeniedHosts []string

	// AdditionalAllowedPorts adds explicitly trusted ports to the normal HTTP
	// and HTTPS ports. By default, http is restricted to 80 and https to 443.
	AdditionalAllowedPorts []int
}

// Policy is safe for concurrent use after construction.
type Policy struct {
	resolver             Resolver
	allowPrivateNetworks bool
	allowedHosts         []hostRule
	deniedHosts          []hostRule
	additionalPorts      map[uint16]struct{}
}

// Target is the validated, normalized URL and the exact addresses returned by
// the resolver during validation. Addresses is a defensive copy.
type Target struct {
	URL       *url.URL
	Host      string
	Port      uint16
	Addresses []netip.Addr
}

type hostRule struct {
	all      bool
	wildcard bool
	host     string
}

// NewPolicy validates options and constructs an immutable outbound policy.
func NewPolicy(options Options) (*Policy, error) {
	policy := &Policy{
		resolver:             options.Resolver,
		allowPrivateNetworks: options.AllowPrivateNetworks,
		additionalPorts:      make(map[uint16]struct{}, len(options.AdditionalAllowedPorts)),
	}
	if policy.resolver == nil {
		policy.resolver = net.DefaultResolver
	}

	var err error
	policy.allowedHosts, err = parseHostRules(options.AllowedHosts, false)
	if err != nil {
		return nil, err
	}
	policy.deniedHosts, err = parseHostRules(options.DeniedHosts, true)
	if err != nil {
		return nil, err
	}

	for _, port := range options.AdditionalAllowedPorts {
		if port < 1 || port > 65535 {
			return nil, fmt.Errorf("%w: port must be between 1 and 65535", ErrInvalidPolicy)
		}
		policy.additionalPorts[uint16(port)] = struct{}{}
	}
	return policy, nil
}

// ParseAndValidate parses rawURL and applies the complete policy.
func (p *Policy) ParseAndValidate(ctx context.Context, rawURL string) (*Target, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: policy is nil", ErrInvalidPolicy)
	}
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return nil, fmt.Errorf("%w: malformed URL", ErrInvalidURL)
	}
	return p.Validate(ctx, parsed)
}

// Validate applies scheme, authority, host-rule, port, and resolved-address
// checks. Callers must still use a policy-aware dialer: validation alone does
// not prevent DNS rebinding between this lookup and a later ordinary dial.
func (p *Policy) Validate(ctx context.Context, candidate *url.URL) (*Target, error) {
	if p == nil {
		return nil, fmt.Errorf("%w: policy is nil", ErrInvalidPolicy)
	}
	if candidate == nil {
		return nil, fmt.Errorf("%w: URL is nil", ErrInvalidURL)
	}
	if candidate.User != nil {
		return nil, fmt.Errorf("%w: credentials in URLs are forbidden", ErrInvalidURL)
	}
	if candidate.Fragment != "" || candidate.RawFragment != "" {
		return nil, fmt.Errorf("%w: fragments are forbidden", ErrInvalidURL)
	}
	if candidate.Opaque != "" {
		return nil, fmt.Errorf("%w: opaque URLs are forbidden", ErrInvalidURL)
	}

	scheme := strings.ToLower(candidate.Scheme)
	if scheme != "http" && scheme != "https" {
		return nil, fmt.Errorf("%w: only http and https are supported", ErrInvalidURL)
	}

	host, explicitPort, err := parseAuthority(candidate.Host)
	if err != nil {
		return nil, err
	}
	host, err = canonicalHost(host)
	if err != nil {
		return nil, err
	}
	if err := p.validateHostRules(host); err != nil {
		return nil, err
	}

	port := defaultPort(scheme)
	if explicitPort != 0 {
		port = explicitPort
		if !p.portAllowedForScheme(scheme, port) {
			return nil, fmt.Errorf("%w: port %d", ErrPortNotAllowed, port)
		}
	}

	addresses, err := p.resolveHost(ctx, host)
	if err != nil {
		return nil, err
	}

	normalized := *candidate
	normalized.Scheme = scheme
	normalized.Host = normalizedAuthority(host, port, explicitPort != 0)
	normalized.User = nil
	normalized.Fragment = ""
	normalized.RawFragment = ""

	return &Target{
		URL:       &normalized,
		Host:      host,
		Port:      port,
		Addresses: append([]netip.Addr(nil), addresses...),
	}, nil
}

func (p *Policy) validateHostRules(host string) error {
	if matchesAny(p.deniedHosts, host) {
		return fmt.Errorf("%w: host denied by policy", ErrBlockedHost)
	}
	if len(p.allowedHosts) > 0 && !matchesAny(p.allowedHosts, host) {
		return fmt.Errorf("%w: host is not allowlisted", ErrBlockedHost)
	}
	if !p.allowPrivateNetworks && (host == "localhost" || strings.HasSuffix(host, ".localhost")) {
		return fmt.Errorf("%w: localhost names are forbidden", ErrBlockedHost)
	}
	return nil
}

func (p *Policy) resolveHost(ctx context.Context, host string) ([]netip.Addr, error) {
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		if err := p.validateAddress(address); err != nil {
			return nil, err
		}
		return []netip.Addr{address}, nil
	}

	addresses, err := p.resolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrResolution, err)
	}
	if len(addresses) == 0 {
		return nil, fmt.Errorf("%w: resolver returned no addresses", ErrResolution)
	}

	unique := make([]netip.Addr, 0, len(addresses))
	seen := make(map[netip.Addr]struct{}, len(addresses))
	for _, address := range addresses {
		address = address.Unmap()
		if err := p.validateAddress(address); err != nil {
			// Reject the whole DNS answer rather than silently discarding an
			// unsafe record. This prevents address-family fallback surprises.
			return nil, err
		}
		if _, exists := seen[address]; exists {
			continue
		}
		seen[address] = struct{}{}
		unique = append(unique, address)
	}
	if len(unique) == 0 {
		return nil, fmt.Errorf("%w: resolver returned no usable addresses", ErrResolution)
	}
	return unique, nil
}

func (p *Policy) validateAddress(address netip.Addr) error {
	if !address.IsValid() || address.Zone() != "" {
		return fmt.Errorf("%w: invalid or zoned IP address", ErrBlockedAddress)
	}
	if address.IsUnspecified() || address.IsMulticast() {
		return fmt.Errorf("%w: unspecified and multicast addresses are forbidden", ErrBlockedAddress)
	}
	if p.allowPrivateNetworks {
		return nil
	}
	if address.IsPrivate() ||
		address.IsLoopback() ||
		address.IsLinkLocalUnicast() ||
		address.IsLinkLocalMulticast() ||
		!address.IsGlobalUnicast() ||
		isSpecialUseAddress(address) {
		return fmt.Errorf("%w: address is not public unicast", ErrBlockedAddress)
	}
	return nil
}

func (p *Policy) portAllowedForScheme(scheme string, port uint16) bool {
	if port == defaultPort(scheme) {
		return true
	}
	_, allowed := p.additionalPorts[port]
	return allowed
}

func (p *Policy) portAllowedForDial(port uint16) bool {
	if port == 80 || port == 443 {
		return true
	}
	_, allowed := p.additionalPorts[port]
	return allowed
}

func parseAuthority(authority string) (string, uint16, error) {
	if authority == "" {
		return "", 0, fmt.Errorf("%w: host is required", ErrInvalidURL)
	}

	if strings.HasPrefix(authority, "[") {
		closing := strings.LastIndexByte(authority, ']')
		if closing < 0 {
			return "", 0, fmt.Errorf("%w: malformed IPv6 authority", ErrInvalidURL)
		}
		host := authority[1:closing]
		remainder := authority[closing+1:]
		if remainder == "" {
			return host, 0, nil
		}
		if !strings.HasPrefix(remainder, ":") || len(remainder) == 1 {
			return "", 0, fmt.Errorf("%w: malformed authority", ErrInvalidURL)
		}
		port, err := parsePort(remainder[1:])
		return host, port, err
	}

	switch strings.Count(authority, ":") {
	case 0:
		return authority, 0, nil
	case 1:
		host, rawPort, _ := strings.Cut(authority, ":")
		if host == "" || rawPort == "" {
			return "", 0, fmt.Errorf("%w: malformed authority", ErrInvalidURL)
		}
		port, err := parsePort(rawPort)
		return host, port, err
	default:
		return "", 0, fmt.Errorf("%w: IPv6 addresses must be bracketed", ErrInvalidURL)
	}
}

func parsePort(raw string) (uint16, error) {
	value, err := strconv.ParseUint(raw, 10, 16)
	if err != nil || value == 0 {
		return 0, fmt.Errorf("%w: invalid port", ErrInvalidURL)
	}
	return uint16(value), nil
}

func defaultPort(scheme string) uint16 {
	if scheme == "https" {
		return 443
	}
	return 80
}

func normalizedAuthority(host string, port uint16, includePort bool) string {
	if !includePort {
		if strings.Contains(host, ":") {
			return "[" + host + "]"
		}
		return host
	}
	return net.JoinHostPort(host, strconv.Itoa(int(port)))
}

func canonicalHost(host string) (string, error) {
	host = strings.TrimSuffix(strings.TrimSpace(host), ".")
	if host == "" {
		return "", fmt.Errorf("%w: host is required", ErrInvalidURL)
	}
	if strings.ContainsAny(host, "%/\\\x00") {
		return "", fmt.Errorf("%w: malformed host", ErrInvalidURL)
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.Unmap().String(), nil
	}

	host = strings.ToLower(host)
	if !validDNSName(host) {
		return "", fmt.Errorf("%w: malformed hostname", ErrInvalidURL)
	}
	return host, nil
}

func validDNSName(host string) bool {
	if len(host) == 0 || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, character := range label {
			if (character >= 'a' && character <= 'z') ||
				(character >= '0' && character <= '9') ||
				character == '-' {
				continue
			}
			return false
		}
	}
	return true
}

func parseHostRules(rawRules []string, allowAll bool) ([]hostRule, error) {
	rules := make([]hostRule, 0, len(rawRules))
	for _, raw := range rawRules {
		value := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
		if value == "" {
			return nil, fmt.Errorf("%w: empty host rule", ErrInvalidPolicy)
		}
		if value == "*" {
			if !allowAll {
				return nil, fmt.Errorf("%w: wildcard allow rule is redundant and forbidden", ErrInvalidPolicy)
			}
			rules = append(rules, hostRule{all: true})
			continue
		}

		wildcard := strings.HasPrefix(value, "*.")
		if wildcard {
			value = strings.TrimPrefix(value, "*.")
			if _, err := netip.ParseAddr(value); err == nil {
				return nil, fmt.Errorf("%w: wildcard IP host rule", ErrInvalidPolicy)
			}
		} else if strings.Contains(value, "*") {
			return nil, fmt.Errorf("%w: wildcard is only supported as a leading *.", ErrInvalidPolicy)
		}

		host, err := canonicalHost(value)
		if err != nil {
			return nil, fmt.Errorf("%w: malformed host rule", ErrInvalidPolicy)
		}
		rules = append(rules, hostRule{wildcard: wildcard, host: host})
	}
	return rules, nil
}

func matchesAny(rules []hostRule, host string) bool {
	for _, rule := range rules {
		if rule.all {
			return true
		}
		if rule.wildcard {
			if strings.HasSuffix(host, "."+rule.host) {
				return true
			}
			continue
		}
		if host == rule.host {
			return true
		}
	}
	return false
}

var blockedSpecialUsePrefixes = []netip.Prefix{
	netip.MustParsePrefix("0.0.0.0/8"),
	netip.MustParsePrefix("100.64.0.0/10"),
	netip.MustParsePrefix("192.0.0.0/24"),
	netip.MustParsePrefix("192.0.2.0/24"),
	netip.MustParsePrefix("198.18.0.0/15"),
	netip.MustParsePrefix("198.51.100.0/24"),
	netip.MustParsePrefix("203.0.113.0/24"),
	netip.MustParsePrefix("240.0.0.0/4"),
	netip.MustParsePrefix("::/96"),
	netip.MustParsePrefix("64:ff9b::/96"),
	netip.MustParsePrefix("64:ff9b:1::/48"),
	netip.MustParsePrefix("100::/64"),
	netip.MustParsePrefix("2001::/32"),
	netip.MustParsePrefix("2001:2::/48"),
	netip.MustParsePrefix("2001:10::/28"),
	netip.MustParsePrefix("2001:db8::/32"),
	netip.MustParsePrefix("2002::/16"),
}

func isSpecialUseAddress(address netip.Addr) bool {
	for _, prefix := range blockedSpecialUsePrefixes {
		if prefix.Contains(address) {
			return true
		}
	}
	return false
}
