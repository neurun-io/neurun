package function

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var (
	ErrInvalidManifest = errors.New("invalid function manifest")
	ErrDigestMismatch  = errors.New("manifest digest mismatch")
	ErrInvalidBundle   = errors.New("invalid function manifest bundle")
)

var (
	functionNamePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*(?:\.[a-z][a-z0-9_]*)+$`)
	categoryPattern     = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	versionPattern      = regexp.MustCompile(`^(?:0|[1-9][0-9]*)(?:\.(?:0|[1-9][0-9]*)){0,2}(?:-[0-9A-Za-z]+(?:[.-][0-9A-Za-z]+)*)?$`)
	tokenPattern        = regexp.MustCompile(`^[a-z][a-z0-9_.:-]*$`)
	digestPattern       = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

// ExecutionRequirement declares the bounded runtime context a function needs.
type ExecutionRequirement string

const (
	ExecutionContextNone             ExecutionRequirement = "none"
	ExecutionContextHTTPAttempt      ExecutionRequirement = "http_attempt"
	ExecutionContextBrowserAttempt   ExecutionRequirement = "browser_attempt"
	ExecutionContextExistingSession  ExecutionRequirement = "existing_session"
	ExecutionContextBrowserOrSession ExecutionRequirement = "browser_or_session"
)

// SideEffectClass controls whether an invocation is safe to retry after an
// ambiguous failure.
type SideEffectClass string

const (
	SideEffectPure          SideEffectClass = "pure"
	SideEffectIdempotent    SideEffectClass = "idempotent"
	SideEffectNonIdempotent SideEffectClass = "non_idempotent"
)

type TimeoutPolicy struct {
	DefaultMS int64 `json:"default_ms"`
	MaximumMS int64 `json:"maximum_ms"`
}

type ResourcePolicy struct {
	MemoryBytes     int64 `json:"memory_bytes,omitempty"`
	CPUMilliseconds int64 `json:"cpu_ms,omitempty"`
	NetworkBytes    int64 `json:"network_bytes,omitempty"`
	ArtifactBytes   int64 `json:"artifact_bytes,omitempty"`
	MaxProcesses    int   `json:"max_processes,omitempty"`
}

type ArtifactPolicy struct {
	Produces []string `json:"produces,omitempty"`
	Required bool     `json:"required,omitempty"`
}

type RedactionPolicy struct {
	SecretFields       []string `json:"secret_fields,omitempty"`
	OutputSecretFields []string `json:"output_secret_fields,omitempty"`
	RedactArtifacts    bool     `json:"redact_artifacts,omitempty"`
}

type RetryPolicy struct {
	AllowedFailures []FailureCategory `json:"allowed_failures,omitempty"`
}

type TelemetryPolicy struct {
	Dimensions []string `json:"dimensions,omitempty"`
}

// Manifest is the published contract for one immutable built-in version. Use
// NormalizeManifest to seal a definition with its content digest before
// registration.
type Manifest struct {
	Name             string               `json:"name"`
	Version          string               `json:"version"`
	Digest           string               `json:"digest"`
	Category         string               `json:"category"`
	Description      string               `json:"description,omitempty"`
	ExecutionContext ExecutionRequirement `json:"execution_context"`
	SideEffects      SideEffectClass      `json:"side_effects"`
	Timeout          TimeoutPolicy        `json:"timeout"`
	Capabilities     []string             `json:"capabilities,omitempty"`
	Permissions      []string             `json:"permissions,omitempty"`
	InputSchema      Schema               `json:"input_schema"`
	OutputSchema     Schema               `json:"output_schema"`
	Resources        ResourcePolicy       `json:"resource_policy,omitempty"`
	Artifacts        ArtifactPolicy       `json:"artifacts,omitempty"`
	Redaction        RedactionPolicy      `json:"redaction,omitempty"`
	Retry            RetryPolicy          `json:"retry,omitempty"`
	Telemetry        TelemetryPolicy      `json:"telemetry,omitempty"`
}

// FunctionRef pins a function to an exact immutable version and digest.
type FunctionRef struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Digest  string `json:"digest"`
}

func (m Manifest) Ref() FunctionRef {
	return FunctionRef{Name: m.Name, Version: m.Version, Digest: m.Digest}
}

// Clone returns a deep copy so callers cannot mutate registry-owned contract
// slices or maps.
func (m Manifest) Clone() Manifest {
	clone, err := cloneManifest(m)
	if err != nil {
		panic(fmt.Sprintf("clone validated function manifest: %v", err))
	}
	return clone
}

// Validate verifies the complete immutable manifest, including its digest.
func (m Manifest) Validate() error {
	if m.Digest == "" {
		return fmt.Errorf("%w: digest is required", ErrInvalidManifest)
	}
	normalized, err := normalizeManifestDefinition(m)
	if err != nil {
		return err
	}
	expected, err := digestManifestDefinition(normalized)
	if err != nil {
		return err
	}
	if !digestPattern.MatchString(m.Digest) {
		return fmt.Errorf("%w: digest must be lowercase sha256:<64 hex>", ErrInvalidManifest)
	}
	if m.Digest != expected {
		return fmt.Errorf("%w: got %s, expected %s", ErrDigestMismatch, m.Digest, expected)
	}
	return nil
}

// NormalizeManifest validates, deep-copies, deterministically orders and seals
// a manifest. If a digest is already present, it must match.
func NormalizeManifest(manifest Manifest) (Manifest, error) {
	providedDigest := manifest.Digest
	normalized, err := normalizeManifestDefinition(manifest)
	if err != nil {
		return Manifest{}, err
	}
	digest, err := digestManifestDefinition(normalized)
	if err != nil {
		return Manifest{}, err
	}
	if providedDigest != "" && providedDigest != digest {
		return Manifest{}, fmt.Errorf("%w: got %s, expected %s", ErrDigestMismatch, providedDigest, digest)
	}
	normalized.Digest = digest
	return normalized, nil
}

// DigestManifest returns the normalized content digest without modifying the
// supplied value.
func DigestManifest(manifest Manifest) (string, error) {
	normalized, err := normalizeManifestDefinition(manifest)
	if err != nil {
		return "", err
	}
	return digestManifestDefinition(normalized)
}

// RetryAllowed applies both the manifest failure allowlist and side-effect
// declaration. Non-idempotent functions are never automatically retried after
// an ambiguous invocation.
func (m Manifest) RetryAllowed(category FailureCategory) bool {
	if m.SideEffects == SideEffectNonIdempotent {
		return false
	}
	for _, allowed := range m.Retry.AllowedFailures {
		if allowed == category {
			return true
		}
	}
	return false
}

func normalizeManifestDefinition(manifest Manifest) (Manifest, error) {
	normalized, err := cloneManifest(manifest)
	if err != nil {
		return Manifest{}, fmt.Errorf("%w: cannot clone: %v", ErrInvalidManifest, err)
	}
	normalized.Digest = ""

	var problems []string
	if !functionNamePattern.MatchString(normalized.Name) {
		problems = append(problems, "name must contain at least two lowercase dot-separated segments")
	}
	if !versionPattern.MatchString(normalized.Version) {
		problems = append(problems, "version must be an integer or semantic version, optionally with a prerelease")
	}
	if !categoryPattern.MatchString(normalized.Category) {
		problems = append(problems, "category must be a lowercase token")
	}
	switch normalized.ExecutionContext {
	case ExecutionContextNone, ExecutionContextHTTPAttempt, ExecutionContextBrowserAttempt,
		ExecutionContextExistingSession, ExecutionContextBrowserOrSession:
	default:
		problems = append(problems, fmt.Sprintf("unsupported execution_context %q", normalized.ExecutionContext))
	}
	switch normalized.SideEffects {
	case SideEffectPure, SideEffectIdempotent, SideEffectNonIdempotent:
	default:
		problems = append(problems, fmt.Sprintf("unsupported side_effects %q", normalized.SideEffects))
	}
	if normalized.Timeout.DefaultMS <= 0 {
		problems = append(problems, "timeout.default_ms must be positive")
	}
	if normalized.Timeout.MaximumMS <= 0 {
		problems = append(problems, "timeout.maximum_ms must be positive")
	}
	if normalized.Timeout.DefaultMS > normalized.Timeout.MaximumMS {
		problems = append(problems, "timeout.default_ms cannot exceed maximum_ms")
	}

	normalizeTokenSet("capabilities", &normalized.Capabilities, &problems)
	normalizeTokenSet("permissions", &normalized.Permissions, &problems)
	normalizeTokenSet("artifacts.produces", &normalized.Artifacts.Produces, &problems)
	normalizeTokenSet("telemetry.dimensions", &normalized.Telemetry.Dimensions, &problems)
	normalizePathSet("redaction.secret_fields", &normalized.Redaction.SecretFields, &problems)
	normalizePathSet("redaction.output_secret_fields", &normalized.Redaction.OutputSecretFields, &problems)

	allowedFailures := make([]string, len(normalized.Retry.AllowedFailures))
	for i, category := range normalized.Retry.AllowedFailures {
		allowedFailures[i] = string(category)
	}
	normalizeTokenSet("retry.allowed_failures", &allowedFailures, &problems)
	normalized.Retry.AllowedFailures = make([]FailureCategory, len(allowedFailures))
	for i, category := range allowedFailures {
		normalized.Retry.AllowedFailures[i] = FailureCategory(category)
	}

	if err := normalized.InputSchema.ValidateDefinition(); err != nil {
		problems = append(problems, "input_schema: "+err.Error())
	}
	if err := normalized.OutputSchema.ValidateDefinition(); err != nil {
		problems = append(problems, "output_schema: "+err.Error())
	}
	normalizeSchema(&normalized.InputSchema, &problems)
	normalizeSchema(&normalized.OutputSchema, &problems)

	if normalized.Resources.MemoryBytes < 0 ||
		normalized.Resources.CPUMilliseconds < 0 ||
		normalized.Resources.NetworkBytes < 0 ||
		normalized.Resources.ArtifactBytes < 0 ||
		normalized.Resources.MaxProcesses < 0 {
		problems = append(problems, "resource_policy values cannot be negative")
	}

	if len(problems) != 0 {
		return Manifest{}, fmt.Errorf("%w: %s", ErrInvalidManifest, strings.Join(problems, "; "))
	}
	return normalized, nil
}

func normalizeTokenSet(field string, values *[]string, problems *[]string) {
	seen := make(map[string]struct{}, len(*values))
	for _, value := range *values {
		if !tokenPattern.MatchString(value) {
			*problems = append(*problems, fmt.Sprintf("%s contains invalid token %q", field, value))
		}
		if _, ok := seen[value]; ok {
			*problems = append(*problems, fmt.Sprintf("%s contains duplicate %q", field, value))
		}
		seen[value] = struct{}{}
	}
	sort.Strings(*values)
}

func normalizePathSet(field string, values *[]string, problems *[]string) {
	seen := make(map[string]struct{}, len(*values))
	for _, value := range *values {
		if value == "" || strings.TrimSpace(value) != value {
			*problems = append(*problems, fmt.Sprintf("%s contains invalid path %q", field, value))
		}
		if _, ok := seen[value]; ok {
			*problems = append(*problems, fmt.Sprintf("%s contains duplicate %q", field, value))
		}
		seen[value] = struct{}{}
	}
	sort.Strings(*values)
}

func normalizeSchema(schema *Schema, problems *[]string) {
	sort.Strings(schema.Required)
	if schema.Enum != nil {
		sort.Slice(schema.Enum, func(i, j int) bool {
			left, leftErr := canonicalJSON(schema.Enum[i])
			right, rightErr := canonicalJSON(schema.Enum[j])
			if leftErr != nil || rightErr != nil {
				return i < j
			}
			return bytes.Compare(left, right) < 0
		})
	}
	keys := make([]string, 0, len(schema.Properties))
	for key := range schema.Properties {
		keys = append(keys, key)
	}
	for _, key := range keys {
		child := schema.Properties[key]
		normalizeSchema(&child, problems)
		schema.Properties[key] = child
	}
	if schema.Items != nil {
		normalizeSchema(schema.Items, problems)
	}
}

func digestManifestDefinition(manifest Manifest) (string, error) {
	type digestPayload Manifest
	payload := digestPayload(manifest)
	payload.Digest = ""
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode digest payload: %v", ErrInvalidManifest, err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func cloneManifest(manifest Manifest) (Manifest, error) {
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return Manifest{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var clone Manifest
	if err := decoder.Decode(&clone); err != nil {
		return Manifest{}, err
	}
	return clone, nil
}

// ManifestBundle is a release artifact containing normalized immutable
// manifests. Digest and signature are bundle-level metadata; signatures are
// produced by the release pipeline and are intentionally optional in-process.
type ManifestBundle struct {
	SchemaVersion string     `json:"schema_version"`
	BundleVersion string     `json:"bundle_version"`
	Digest        string     `json:"digest"`
	Signature     string     `json:"signature,omitempty"`
	Manifests     []Manifest `json:"manifests"`
}

// NormalizeBundle validates and deterministically orders a complete bundle.
func NormalizeBundle(bundleVersion string, manifests []Manifest) (ManifestBundle, error) {
	if !versionPattern.MatchString(bundleVersion) {
		return ManifestBundle{}, fmt.Errorf("%w: invalid bundle_version %q", ErrInvalidBundle, bundleVersion)
	}
	bundle := ManifestBundle{
		SchemaVersion: "1",
		BundleVersion: bundleVersion,
		Manifests:     make([]Manifest, len(manifests)),
	}
	seen := make(map[string]struct{}, len(manifests))
	for i, manifest := range manifests {
		normalized, err := NormalizeManifest(manifest)
		if err != nil {
			return ManifestBundle{}, fmt.Errorf("%w: manifest %d: %w", ErrInvalidBundle, i, err)
		}
		key := normalized.Name + "@" + normalized.Version
		if _, ok := seen[key]; ok {
			return ManifestBundle{}, fmt.Errorf("%w: duplicate manifest %s", ErrInvalidBundle, key)
		}
		seen[key] = struct{}{}
		bundle.Manifests[i] = normalized
	}
	sort.Slice(bundle.Manifests, func(i, j int) bool {
		if bundle.Manifests[i].Name != bundle.Manifests[j].Name {
			return bundle.Manifests[i].Name < bundle.Manifests[j].Name
		}
		return compareVersions(bundle.Manifests[i].Version, bundle.Manifests[j].Version) < 0
	})
	digest, err := digestBundle(bundle)
	if err != nil {
		return ManifestBundle{}, err
	}
	bundle.Digest = digest
	return bundle, nil
}

func (b ManifestBundle) Validate() error {
	normalized, err := NormalizeBundle(b.BundleVersion, b.Manifests)
	if err != nil {
		return err
	}
	if b.SchemaVersion != "1" {
		return fmt.Errorf("%w: unsupported schema_version %q", ErrInvalidBundle, b.SchemaVersion)
	}
	if !digestPattern.MatchString(b.Digest) {
		return fmt.Errorf("%w: invalid digest", ErrInvalidBundle)
	}
	if b.Digest != normalized.Digest {
		return fmt.Errorf("%w: got %s, expected %s", ErrDigestMismatch, b.Digest, normalized.Digest)
	}
	return nil
}

// MarshalNormalized emits the compact, stable release representation.
func (b ManifestBundle) MarshalNormalized() ([]byte, error) {
	normalized, err := NormalizeBundle(b.BundleVersion, b.Manifests)
	if err != nil {
		return nil, err
	}
	normalized.Signature = b.Signature
	return json.Marshal(normalized)
}

func digestBundle(bundle ManifestBundle) (string, error) {
	payload := struct {
		SchemaVersion string     `json:"schema_version"`
		BundleVersion string     `json:"bundle_version"`
		Manifests     []Manifest `json:"manifests"`
	}{
		SchemaVersion: bundle.SchemaVersion,
		BundleVersion: bundle.BundleVersion,
		Manifests:     bundle.Manifests,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("%w: encode digest payload: %v", ErrInvalidBundle, err)
	}
	sum := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}
