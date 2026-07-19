package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	AliasStable = "stable"
	AliasLatest = "latest"
)

var (
	ErrFunctionNotFound  = errors.New("atomic function not found")
	ErrFunctionExists    = errors.New("atomic function version already registered")
	ErrAliasNotFound     = errors.New("atomic function alias not found")
	ErrDigestPinMismatch = errors.New("atomic function digest pin mismatch")
)

// Registry stores immutable snapshots of compiled-in atomic functions.
type Registry struct {
	mu        sync.RWMutex
	functions map[string]map[string]*registeredFunction
	aliases   map[string]map[string]string
	digests   map[string]string
}

func NewRegistry() *Registry {
	return &Registry{
		functions: make(map[string]map[string]*registeredFunction),
		aliases:   make(map[string]map[string]string),
		digests:   make(map[string]string),
	}
}

type registeredFunction struct {
	manifest Manifest
	delegate AtomicFunction
}

func (f *registeredFunction) Manifest() Manifest {
	return f.manifest.Clone()
}

func (f *registeredFunction) Execute(
	ctx context.Context,
	execution *ExecutionContext,
	input json.RawMessage,
) (FunctionResult, error) {
	return f.delegate.Execute(ctx, execution, append(json.RawMessage(nil), input...))
}

// Register adds one exact immutable function version and atomically advances
// the derived latest/stable aliases.
func (r *Registry) Register(function AtomicFunction) error {
	if function == nil {
		return errors.New("register atomic function: function is nil")
	}
	manifest, err := NormalizeManifest(function.Manifest())
	if err != nil {
		return fmt.Errorf("register atomic function: %w", err)
	}
	key := manifest.Name + "@" + manifest.Version

	r.mu.Lock()
	defer r.mu.Unlock()

	versions := r.functions[manifest.Name]
	if versions == nil {
		versions = make(map[string]*registeredFunction)
		r.functions[manifest.Name] = versions
	}
	if _, exists := versions[manifest.Version]; exists {
		return fmt.Errorf("%w: %s", ErrFunctionExists, key)
	}
	if existing, exists := r.digests[manifest.Digest]; exists {
		return fmt.Errorf("%w: digest %s already belongs to %s", ErrFunctionExists, manifest.Digest, existing)
	}
	versions[manifest.Version] = &registeredFunction{manifest: manifest, delegate: function}
	r.digests[manifest.Digest] = key

	aliases := r.aliases[manifest.Name]
	if aliases == nil {
		aliases = make(map[string]string)
		r.aliases[manifest.Name] = aliases
	}
	if latest, ok := aliases[AliasLatest]; !ok || compareVersions(manifest.Version, latest) > 0 {
		aliases[AliasLatest] = manifest.Version
	}
	if !isPrerelease(manifest.Version) {
		if stable, ok := aliases[AliasStable]; !ok || compareVersions(manifest.Version, stable) > 0 {
			aliases[AliasStable] = manifest.Version
		}
	}
	return nil
}

// RegisterAll registers a bundle atomically with respect to readers. On an
// error, no function from the supplied slice remains registered.
func (r *Registry) RegisterAll(functions ...AtomicFunction) error {
	if len(functions) == 0 {
		return nil
	}
	staging := NewRegistry()
	for _, function := range functions {
		if err := staging.Register(function); err != nil {
			return err
		}
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	for name, stagedVersions := range staging.functions {
		existingVersions := r.functions[name]
		for version, staged := range stagedVersions {
			if existingVersions != nil {
				if _, exists := existingVersions[version]; exists {
					return fmt.Errorf("%w: %s@%s", ErrFunctionExists, name, version)
				}
			}
			if existing, exists := r.digests[staged.manifest.Digest]; exists {
				return fmt.Errorf("%w: digest %s already belongs to %s", ErrFunctionExists, staged.manifest.Digest, existing)
			}
		}
	}
	for name, stagedVersions := range staging.functions {
		versions := r.functions[name]
		if versions == nil {
			versions = make(map[string]*registeredFunction)
			r.functions[name] = versions
		}
		for version, staged := range stagedVersions {
			versions[version] = staged
			r.digests[staged.manifest.Digest] = name + "@" + version
		}
		r.rebuildAliasesLocked(name)
	}
	return nil
}

// SetAlias pins a named alias to an already registered exact version. The
// reserved stable/latest aliases may be explicitly reset and will advance
// again as newer versions are registered.
func (r *Registry) SetAlias(name, alias, version string) error {
	if !tokenPattern.MatchString(alias) {
		return fmt.Errorf("invalid alias %q", alias)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.functions[name]
	if versions == nil {
		return fmt.Errorf("%w: %s", ErrFunctionNotFound, name)
	}
	if _, ok := versions[version]; !ok {
		return fmt.Errorf("%w: %s@%s", ErrFunctionNotFound, name, version)
	}
	if _, exactCollision := versions[alias]; exactCollision {
		return fmt.Errorf("alias %q collides with an exact version", alias)
	}
	if r.aliases[name] == nil {
		r.aliases[name] = make(map[string]string)
	}
	r.aliases[name][alias] = version
	return nil
}

// Resolve accepts an exact version or an alias. An empty version resolves the
// stable alias.
func (r *Registry) Resolve(name, versionOrAlias string) (AtomicFunction, error) {
	_, function, err := r.ResolveRef(FunctionRef{Name: name, Version: versionOrAlias})
	return function, err
}

// ResolveRef resolves aliases and verifies an optional digest pin.
func (r *Registry) ResolveRef(requested FunctionRef) (FunctionRef, AtomicFunction, error) {
	version := requested.Version
	if version == "" {
		version = AliasStable
	}

	r.mu.RLock()
	defer r.mu.RUnlock()
	versions := r.functions[requested.Name]
	if versions == nil {
		return FunctionRef{}, nil, fmt.Errorf("%w: %s", ErrFunctionNotFound, requested.Name)
	}
	function := versions[version]
	if function == nil {
		exact, aliasFound := r.aliases[requested.Name][version]
		if !aliasFound {
			if version == AliasStable || version == AliasLatest {
				return FunctionRef{}, nil, fmt.Errorf("%w: %s@%s", ErrAliasNotFound, requested.Name, version)
			}
			return FunctionRef{}, nil, fmt.Errorf("%w: %s@%s", ErrFunctionNotFound, requested.Name, version)
		}
		function = versions[exact]
	}
	resolved := function.manifest.Ref()
	if requested.Digest != "" && requested.Digest != resolved.Digest {
		return FunctionRef{}, nil, fmt.Errorf(
			"%w: requested %s, resolved %s", ErrDigestPinMismatch, requested.Digest, resolved.Digest,
		)
	}
	return resolved, function, nil
}

func (r *Registry) Manifest(name, versionOrAlias string) (Manifest, error) {
	function, err := r.Resolve(name, versionOrAlias)
	if err != nil {
		return Manifest{}, err
	}
	return function.Manifest(), nil
}

// List returns exact versions in deterministic name and semantic-version
// order. Returned manifests are independent deep copies.
func (r *Registry) List() []Manifest {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifests := make([]Manifest, 0, len(r.digests))
	for _, versions := range r.functions {
		for _, function := range versions {
			manifests = append(manifests, function.manifest.Clone())
		}
	}
	sort.Slice(manifests, func(i, j int) bool {
		if manifests[i].Name != manifests[j].Name {
			return manifests[i].Name < manifests[j].Name
		}
		return compareVersions(manifests[i].Version, manifests[j].Version) < 0
	})
	return manifests
}

func (r *Registry) Bundle(bundleVersion string) (ManifestBundle, error) {
	return NormalizeBundle(bundleVersion, r.List())
}

func (r *Registry) rebuildAliasesLocked(name string) {
	versions := r.functions[name]
	aliases := r.aliases[name]
	if aliases == nil {
		aliases = make(map[string]string)
		r.aliases[name] = aliases
	}
	delete(aliases, AliasLatest)
	delete(aliases, AliasStable)
	for version := range versions {
		if latest, ok := aliases[AliasLatest]; !ok || compareVersions(version, latest) > 0 {
			aliases[AliasLatest] = version
		}
		if !isPrerelease(version) {
			if stable, ok := aliases[AliasStable]; !ok || compareVersions(version, stable) > 0 {
				aliases[AliasStable] = version
			}
		}
	}
}

type parsedVersion struct {
	core       [3]uint64
	coreLength int
	prerelease []string
	raw        string
}

func parseVersion(version string) parsedVersion {
	parsed := parsedVersion{raw: version}
	parts := strings.SplitN(version, "-", 2)
	for index, part := range strings.Split(parts[0], ".") {
		if index >= len(parsed.core) {
			break
		}
		value, _ := strconv.ParseUint(part, 10, 64)
		parsed.core[index] = value
		parsed.coreLength++
	}
	if len(parts) == 2 {
		parsed.prerelease = strings.Split(parts[1], ".")
	}
	return parsed
}

func compareVersions(left, right string) int {
	a := parseVersion(left)
	b := parseVersion(right)
	for i := 0; i < len(a.core); i++ {
		if a.core[i] < b.core[i] {
			return -1
		}
		if a.core[i] > b.core[i] {
			return 1
		}
	}
	if len(a.prerelease) == 0 && len(b.prerelease) != 0 {
		return 1
	}
	if len(a.prerelease) != 0 && len(b.prerelease) == 0 {
		return -1
	}
	for i := 0; i < len(a.prerelease) && i < len(b.prerelease); i++ {
		if a.prerelease[i] == b.prerelease[i] {
			continue
		}
		aNumber, aErr := strconv.ParseUint(a.prerelease[i], 10, 64)
		bNumber, bErr := strconv.ParseUint(b.prerelease[i], 10, 64)
		switch {
		case aErr == nil && bErr == nil:
			if aNumber < bNumber {
				return -1
			}
			return 1
		case aErr == nil:
			return -1
		case bErr == nil:
			return 1
		default:
			if a.prerelease[i] < b.prerelease[i] {
				return -1
			}
			return 1
		}
	}
	if len(a.prerelease) < len(b.prerelease) {
		return -1
	}
	if len(a.prerelease) > len(b.prerelease) {
		return 1
	}
	// 1, 1.0 and 1.0.0 have equal semantic precedence but remain distinct
	// immutable versions. Prefer the more explicit representation, then lexical
	// order, so alias resolution remains deterministic.
	if a.coreLength < b.coreLength {
		return -1
	}
	if a.coreLength > b.coreLength {
		return 1
	}
	return strings.Compare(a.raw, b.raw)
}

func isPrerelease(version string) bool {
	return strings.Contains(version, "-")
}
