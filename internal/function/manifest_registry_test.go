package function

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type testAtomic struct {
	manifest Manifest
	execute  Executor
}

func (f *testAtomic) Manifest() Manifest {
	return f.manifest
}

func (f *testAtomic) Execute(
	ctx context.Context,
	execution *ExecutionContext,
	input json.RawMessage,
) (FunctionResult, error) {
	if f.execute != nil {
		return f.execute(ctx, execution, input)
	}
	return FunctionResult{Output: json.RawMessage(`null`)}, nil
}

func baseTestManifest(name, version string) Manifest {
	return Manifest{
		Name:             name,
		Version:          version,
		Category:         "test",
		ExecutionContext: ExecutionContextNone,
		SideEffects:      SideEffectPure,
		Timeout:          TimeoutPolicy{DefaultMS: 100, MaximumMS: 1000},
		InputSchema:      Schema{},
		OutputSchema:     Schema{},
	}
}

func TestNormalizeManifestDigestIsDeterministic(t *testing.T) {
	t.Parallel()
	left := baseTestManifest("test.digest", "1")
	left.Capabilities = []string{"zeta", "alpha"}
	left.Permissions = []string{"write.artifact", "read.page"}
	left.Retry.AllowedFailures = []FailureCategory{FailureTransientNetwork, FailureAgentLost}
	left.InputSchema = Schema{
		Type:     TypeObject,
		Required: []string{"z", "a"},
		Properties: map[string]Schema{
			"z": {Type: TypeString, Enum: []any{"two", "one"}},
			"a": {Type: TypeInteger},
		},
	}

	right := baseTestManifest("test.digest", "1")
	right.Capabilities = []string{"alpha", "zeta"}
	right.Permissions = []string{"read.page", "write.artifact"}
	right.Retry.AllowedFailures = []FailureCategory{FailureAgentLost, FailureTransientNetwork}
	right.InputSchema = Schema{
		Type:     TypeObject,
		Required: []string{"a", "z"},
		Properties: map[string]Schema{
			"a": {Type: TypeInteger},
			"z": {Type: TypeString, Enum: []any{"one", "two"}},
		},
	}

	normalizedLeft, err := NormalizeManifest(left)
	if err != nil {
		t.Fatalf("normalize left: %v", err)
	}
	normalizedRight, err := NormalizeManifest(right)
	if err != nil {
		t.Fatalf("normalize right: %v", err)
	}
	if normalizedLeft.Digest != normalizedRight.Digest {
		t.Fatalf("digest depends on set ordering: %s != %s", normalizedLeft.Digest, normalizedRight.Digest)
	}
	if err := normalizedLeft.Validate(); err != nil {
		t.Fatalf("sealed manifest did not validate: %v", err)
	}

	tampered := normalizedLeft.Clone()
	tampered.Description = "changed after sealing"
	if err := tampered.Validate(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected digest mismatch after tamper, got %v", err)
	}
}

func TestManifestValidationRejectsInvalidContracts(t *testing.T) {
	t.Parallel()
	tests := []Manifest{
		baseTestManifest("single", "1"),
		baseTestManifest("Test.upper", "1"),
		baseTestManifest("test.version", "stable"),
		baseTestManifest("test.version", "01"),
		baseTestManifest("test.ok", "1"),
		baseTestManifest("test.ok", "1"),
		baseTestManifest("test.ok", "1"),
	}
	tests[4].Timeout.DefaultMS = 1001
	tests[5].ExecutionContext = "somewhere"
	tests[6].Capabilities = []string{"chromium", "chromium"}

	for i, manifest := range tests {
		if _, err := NormalizeManifest(manifest); !errors.Is(err, ErrInvalidManifest) {
			t.Errorf("case %d: expected invalid manifest, got %v", i, err)
		}
	}
}

func TestManifestDigestMismatchIsRejectedDuringNormalization(t *testing.T) {
	t.Parallel()
	manifest := baseTestManifest("test.pinned", "1")
	manifest.Digest = "sha256:" + string(make([]byte, 64))
	if _, err := NormalizeManifest(manifest); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected digest mismatch, got %v", err)
	}
}

func TestManifestRetryAllowedHonorsSideEffects(t *testing.T) {
	t.Parallel()
	manifest := baseTestManifest("test.retry", "1")
	manifest.Retry.AllowedFailures = []FailureCategory{FailureAgentLost}
	manifest.SideEffects = SideEffectIdempotent
	if !manifest.RetryAllowed(FailureAgentLost) {
		t.Fatal("idempotent allowlisted failure should retry")
	}
	if manifest.RetryAllowed(FailureTransientNetwork) {
		t.Fatal("non-allowlisted failure should not retry")
	}
	manifest.SideEffects = SideEffectNonIdempotent
	if manifest.RetryAllowed(FailureAgentLost) {
		t.Fatal("non-idempotent function must not automatically retry")
	}
}

func TestManifestBundleIsNormalizedAndTamperEvident(t *testing.T) {
	t.Parallel()
	first := baseTestManifest("zeta.function", "1")
	second := baseTestManifest("alpha.function", "2")
	bundle, err := NormalizeBundle("1", []Manifest{first, second})
	if err != nil {
		t.Fatalf("normalize bundle: %v", err)
	}
	if bundle.Manifests[0].Name != "alpha.function" {
		t.Fatalf("bundle is not name sorted: %#v", bundle.Manifests)
	}
	if err := bundle.Validate(); err != nil {
		t.Fatalf("bundle did not validate: %v", err)
	}
	encodedA, err := bundle.MarshalNormalized()
	if err != nil {
		t.Fatal(err)
	}
	encodedB, err := bundle.MarshalNormalized()
	if err != nil {
		t.Fatal(err)
	}
	if string(encodedA) != string(encodedB) {
		t.Fatal("normalized serialization is not deterministic")
	}
	bundle.Manifests[0].Description = "tampered"
	if err := bundle.Validate(); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected tampered bundle digest mismatch, got %v", err)
	}
}

func TestRegistryResolvesExactStableLatestAndCustomAliases(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	for _, version := range []string{"1", "2-beta.1"} {
		if err := registry.Register(&testAtomic{manifest: baseTestManifest("test.alias", version)}); err != nil {
			t.Fatal(err)
		}
	}
	assertResolvedVersion(t, registry, "test.alias", AliasStable, "1")
	assertResolvedVersion(t, registry, "test.alias", AliasLatest, "2-beta.1")

	if err := registry.Register(&testAtomic{manifest: baseTestManifest("test.alias", "2")}); err != nil {
		t.Fatal(err)
	}
	assertResolvedVersion(t, registry, "test.alias", "", "2")
	assertResolvedVersion(t, registry, "test.alias", AliasLatest, "2")
	assertResolvedVersion(t, registry, "test.alias", "1", "1")

	if err := registry.Register(&testAtomic{manifest: baseTestManifest("test.alias", "3-alpha")}); err != nil {
		t.Fatal(err)
	}
	assertResolvedVersion(t, registry, "test.alias", AliasStable, "2")
	assertResolvedVersion(t, registry, "test.alias", AliasLatest, "3-alpha")

	if err := registry.SetAlias("test.alias", "candidate", "3-alpha"); err != nil {
		t.Fatal(err)
	}
	assertResolvedVersion(t, registry, "test.alias", "candidate", "3-alpha")
}

func TestRegistryVerifiesDigestPinAndSnapshotsManifest(t *testing.T) {
	t.Parallel()
	original := baseTestManifest("test.snapshot", "1")
	original.Capabilities = []string{"alpha"}
	original.InputSchema = Schema{
		Type:       TypeObject,
		Properties: map[string]Schema{"value": {Type: TypeString}},
	}
	function := &testAtomic{manifest: original}
	registry := NewRegistry()
	if err := registry.Register(function); err != nil {
		t.Fatal(err)
	}
	first, err := registry.Manifest("test.snapshot", "1")
	if err != nil {
		t.Fatal(err)
	}

	function.manifest.Capabilities[0] = "mutated"
	function.manifest.InputSchema.Properties["value"] = Schema{Type: TypeNumber}
	second, err := registry.Manifest("test.snapshot", "1")
	if err != nil {
		t.Fatal(err)
	}
	if second.Capabilities[0] != "alpha" || second.InputSchema.Properties["value"].Type != TypeString {
		t.Fatalf("registry manifest changed through delegate mutation: %#v", second)
	}
	second.Capabilities[0] = "caller-mutated"
	third, _ := registry.Manifest("test.snapshot", "1")
	if third.Capabilities[0] != "alpha" {
		t.Fatal("registry manifest changed through returned clone")
	}

	_, _, err = registry.ResolveRef(FunctionRef{
		Name: "test.snapshot", Version: "1", Digest: "sha256:wrong",
	})
	if !errors.Is(err, ErrDigestPinMismatch) {
		t.Fatalf("expected digest pin mismatch, got %v", err)
	}
	resolved, _, err := registry.ResolveRef(first.Ref())
	if err != nil {
		t.Fatalf("exact digest pin rejected: %v", err)
	}
	if resolved != first.Ref() {
		t.Fatalf("resolved ref changed: %#v != %#v", resolved, first.Ref())
	}
}

func TestRegistryRejectsDuplicateWithoutPartialRegisterAll(t *testing.T) {
	t.Parallel()
	registry := NewRegistry()
	versionOne := &testAtomic{manifest: baseTestManifest("test.atomic", "1")}
	if err := registry.Register(versionOne); err != nil {
		t.Fatal(err)
	}
	err := registry.RegisterAll(
		&testAtomic{manifest: baseTestManifest("test.atomic", "2")},
		versionOne,
	)
	if !errors.Is(err, ErrFunctionExists) {
		t.Fatalf("expected duplicate error, got %v", err)
	}
	if _, err := registry.Resolve("test.atomic", "2"); !errors.Is(err, ErrFunctionNotFound) {
		t.Fatalf("RegisterAll partially changed registry: %v", err)
	}
}

func TestRegistryConcurrentReadersAndWriters(t *testing.T) {
	registry := NewRegistry()
	const count = 32
	var wait sync.WaitGroup
	wait.Add(count * 2)
	for i := 1; i <= count; i++ {
		version := fmt.Sprintf("1.0.%d", i)
		go func() {
			defer wait.Done()
			if err := registry.Register(&testAtomic{
				manifest: baseTestManifest("test.concurrent", version),
			}); err != nil {
				t.Errorf("register %s: %v", version, err)
			}
		}()
		go func() {
			defer wait.Done()
			_ = registry.List()
			_, _ = registry.Resolve("test.concurrent", AliasLatest)
		}()
	}
	wait.Wait()
	if got := len(registry.List()); got != count {
		t.Fatalf("registered %d versions, want %d", got, count)
	}
	assertResolvedVersion(t, registry, "test.concurrent", AliasLatest, "1.0.32")
}

func assertResolvedVersion(t *testing.T, registry *Registry, name, requested, want string) {
	t.Helper()
	manifest, err := registry.Manifest(name, requested)
	if err != nil {
		t.Fatalf("resolve %s@%s: %v", name, requested, err)
	}
	if manifest.Version != want {
		t.Fatalf("resolve %s@%s = %s, want %s", name, requested, manifest.Version, want)
	}
}
