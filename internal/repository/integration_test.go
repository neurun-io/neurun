package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/neurun-io/neurun/internal/domain/deployment"
	"github.com/neurun-io/neurun/internal/domain/execution"
	"github.com/neurun-io/neurun/internal/repository/storage"
	"github.com/neurun-io/neurun/migrations"
)

// These exercise the SQL itself, which nothing else can: every other test in
// the tree stops at the repository boundary. Set NEURUN_TEST_DATABASE_URL to a
// PostgreSQL a test may create and drop schemas in.
//
// Each run gets its own schema and drops it afterwards, so a failed run leaves
// nothing behind for the next one to trip over.
func testPool(t *testing.T) (*pgxpool.Pool, func()) {
	t.Helper()
	databaseURL := strings.TrimSpace(os.Getenv("NEURUN_TEST_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("NEURUN_TEST_DATABASE_URL is not set")
	}
	schema := fmt.Sprintf("neurun_test_%d", time.Now().UnixNano())
	if err := migrations.Apply(databaseURL, schema); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	parsed, err := url.Parse(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()

	ctx := context.Background()
	pool, err := storage.PostgresConnect(ctx, storage.PostgresConfig{
		DSN: parsed.String(), MaxConns: 4,
		ConnMaxLifetime: time.Minute, ConnMaxIdleTime: time.Minute,
	})
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	return pool, func() {
		_, err := pool.Exec(
			ctx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE",
		)
		if err != nil {
			t.Errorf("drop schema %s: %v", schema, err)
		}
		pool.Close()
	}
}

// seedOrganization creates the user and organization every project hangs from.
// Projects reference an organization, which references its owner, so a fixture
// cannot start at the project any more.
func seedOrganization(
	t *testing.T,
	ctx context.Context,
	pool *pgxpool.Pool,
	suffix string,
	now time.Time,
) string {
	t.Helper()
	userID := "usr_" + suffix
	organizationID := "org_" + suffix
	_, err := pool.Exec(
		ctx,
		`INSERT INTO users (id, email, password_hash, disabled, created_at, updated_at)
		 VALUES ($1, $2, $3, false, $4, $4)`,
		userID, suffix+"@example.com",
		"$2a$10$abcdefghijklmnopqrstuvabcdefghijklmnopqrstuvwxyz012345", now,
	)
	if err != nil {
		t.Fatalf("seed user: %v", err)
	}
	_, err = pool.Exec(
		ctx,
		`INSERT INTO organizations (id, owner_user_id, name, created_at, updated_at)
		 VALUES ($1, $2, $3, $4, $4)`,
		organizationID, userID, "Org "+suffix, now,
	)
	if err != nil {
		t.Fatalf("seed organization: %v", err)
	}
	_, err = pool.Exec(
		ctx,
		`INSERT INTO organization_members
		 (organization_id, user_id, role, created_at, updated_at)
		 VALUES ($1, $2, 'admin', $3, $3)`,
		organizationID, userID, now,
	)
	if err != nil {
		t.Fatalf("seed membership: %v", err)
	}
	return organizationID
}

// The full path a deployment takes through storage: project, app, upload,
// build, then an execution claimed and finalized against the pinned build.
func TestDeploymentAndExecutionRoundTrip(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()

	projects, err := NewProjectRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	apps, err := NewAppRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	deployments, err := NewDeploymentRepository(pool)
	if err != nil {
		t.Fatal(err)
	}
	executions, err := NewExecutionRepository(pool)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Microsecond)
	organizationID := seedOrganization(t, ctx, pool, "it", now)
	project, err := deployment.NewProject("prj_it", organizationID, "Integration", now)
	if err != nil {
		t.Fatal(err)
	}
	if project, err = projects.Ensure(ctx, project); err != nil {
		t.Fatalf("ensure project: %v", err)
	}
	// Ensure is idempotent: booting twice must not fail.
	if _, err = projects.Ensure(ctx, project); err != nil {
		t.Fatalf("second ensure: %v", err)
	}

	app, err := deployment.NewApp("app_it", project.ID, "Integration App", now)
	if err != nil {
		t.Fatal(err)
	}
	if app, err = apps.Create(ctx, app); err != nil {
		t.Fatalf("create app: %v", err)
	}

	sourceDigest := strings.Repeat("a", 64)
	codeDigest := strings.Repeat("c", 64)
	record, err := deployment.New(
		"dep_it", project.ID, app.ID, deployment.RuntimePython, "main.py:handler",
		deployment.Artifact{
			ID: "art_src", Kind: deployment.ArtifactSource, Name: "source.zip",
			MediaType: "application/zip", SizeBytes: 10, SHA256: sourceDigest,
			StorageKey: "objects/sha256/aa/" + sourceDigest, CreatedAt: now,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := deployments.Save(ctx, record); err != nil {
		t.Fatalf("save uploaded: %v", err)
	}
	if _, err := record.StartBuild("bld_it", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := deployments.Save(ctx, record); err != nil {
		t.Fatalf("save building: %v", err)
	}
	if err := record.MarkBuildReady(
		"bld_it",
		[]deployment.Artifact{{
			ID: "art_code", Kind: deployment.ArtifactCodeLayer, Name: "code.zip",
			MediaType: "application/zip", SizeBytes: 20, SHA256: codeDigest,
			StorageKey: "objects/sha256/cc/" + codeDigest, CreatedAt: now,
		}},
		now.Add(2*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := deployments.Save(ctx, record); err != nil {
		t.Fatalf("save ready: %v", err)
	}

	loaded, err := deployments.GetByID(ctx, organizationID, record.ID)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if loaded.Status != deployment.StatusReady || len(loaded.Builds) != 1 {
		t.Fatalf("loaded = %#v", loaded)
	}
	// The private blob handle has to survive the JSON column round trip, or the
	// worker could not open the artifact it needs.
	if loaded.Source.StorageKey != record.Source.StorageKey ||
		loaded.Builds[0].Artifacts[0].StorageKey != "objects/sha256/cc/"+codeDigest {
		t.Fatalf("storage handles lost in round trip: %#v", loaded.Source)
	}
	if loaded.Builds[0].Failure != nil {
		t.Fatalf("absent failure read back as %#v", loaded.Builds[0].Failure)
	}

	build, ok := loaded.ReadyBuild()
	if !ok {
		t.Fatal("no ready build after round trip")
	}
	queued, err := execution.New(
		"exe_it", project.ID, loaded.ID, build.ID,
		json.RawMessage(`{"hello":"world"}`), now.Add(3*time.Second),
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := executions.Create(ctx, queued); err != nil {
		t.Fatalf("create execution: %v", err)
	}

	claimed, err := executions.ClaimQueued(ctx, now.Add(4*time.Second))
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID != queued.ID || claimed.Status != execution.StatusRunning {
		t.Fatalf("claimed = %#v", claimed)
	}
	// Nothing is left queued, so a second claim finds none.
	if _, err := executions.ClaimQueued(ctx, now.Add(5*time.Second)); err == nil ||
		!strings.Contains(err.Error(), execution.ErrNoQueued.Error()) {
		t.Fatalf("second claim error = %v", err)
	}

	if err := claimed.Succeed(
		json.RawMessage(`{"ok":true}`), "log line\n", now.Add(6*time.Second),
	); err != nil {
		t.Fatal(err)
	}
	if err := executions.Finalize(ctx, claimed); err != nil {
		t.Fatalf("finalize: %v", err)
	}
	// Finalizing again must lose: the record is no longer the running one.
	if err := executions.Finalize(ctx, claimed); err == nil {
		t.Fatal("stale finalize succeeded")
	}

	final, err := executions.GetByID(ctx, organizationID, queued.ID)
	if err != nil {
		t.Fatalf("read execution: %v", err)
	}
	if final.Status != execution.StatusSucceeded ||
		string(final.Output) != `{"ok":true}` || final.Logs != "log line\n" {
		t.Fatalf("final execution = %#v", final)
	}
}

// A crash leaves rows mid-flight; recovery has to close them out without
// re-running anything.
func TestRecoveryClosesInterruptedWork(t *testing.T) {
	pool, cleanup := testPool(t)
	defer cleanup()
	ctx := context.Background()

	projects, _ := NewProjectRepository(pool)
	apps, _ := NewAppRepository(pool)
	deployments, _ := NewDeploymentRepository(pool)
	executions, _ := NewExecutionRepository(pool)

	now := time.Now().UTC().Truncate(time.Microsecond)
	organizationID := seedOrganization(t, ctx, pool, "rec", now)
	project, err := deployment.NewProject("prj_rec", organizationID, "Recovery", now)
	if err != nil {
		t.Fatal(err)
	}
	if project, err = projects.Ensure(ctx, project); err != nil {
		t.Fatal(err)
	}
	app, err := deployment.NewApp("app_rec", project.ID, "Recovery App", now)
	if err != nil {
		t.Fatal(err)
	}
	if app, err = apps.Create(ctx, app); err != nil {
		t.Fatal(err)
	}

	digest := strings.Repeat("d", 64)
	record, err := deployment.New(
		"dep_rec", project.ID, app.ID, deployment.RuntimePython, "main.py:handler",
		deployment.Artifact{
			ID: "art_rec", Kind: deployment.ArtifactSource, Name: "source.zip",
			MediaType: "application/zip", SizeBytes: 10, SHA256: digest,
			StorageKey: "objects/sha256/dd/" + digest, CreatedAt: now,
		},
		now,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := record.StartBuild("bld_rec", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := deployments.Save(ctx, record); err != nil {
		t.Fatal(err)
	}

	recovered, err := deployments.RecoverBuilding(ctx, now.Add(time.Minute),
		deployment.Failure{Code: "build_interrupted", Message: "restarted"})
	if err != nil || recovered != 1 {
		t.Fatalf("recover builds = %d, %v", recovered, err)
	}
	loaded, err := deployments.GetByID(ctx, organizationID, record.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != deployment.StatusFailed || loaded.Builds[0].Failure == nil {
		t.Fatalf("recovered deployment = %#v", loaded)
	}
	// Idempotent: a second pass finds nothing building.
	if again, err := deployments.RecoverBuilding(ctx, now.Add(2*time.Minute),
		deployment.Failure{Code: "build_interrupted", Message: "restarted"}); err != nil || again != 0 {
		t.Fatalf("second recover = %d, %v", again, err)
	}

	// An execution left running is failed, while a queued one is untouched.
	ready := deployment.Artifact{
		ID: "art_code2", Kind: deployment.ArtifactCodeLayer, Name: "code.zip",
		MediaType: "application/zip", SizeBytes: 20,
		SHA256: strings.Repeat("e", 64), CreatedAt: now,
	}
	ready.StorageKey = "objects/sha256/ee/" + ready.SHA256
	fresh, err := deployment.New(
		"dep_rec2", project.ID, app.ID, deployment.RuntimePython, "main.py:handler",
		deployment.Artifact{
			ID: "art_rec2", Kind: deployment.ArtifactSource, Name: "source.zip",
			MediaType: "application/zip", SizeBytes: 10,
			SHA256: strings.Repeat("f", 64), CreatedAt: now,
		}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	fresh.Source.StorageKey = "objects/sha256/ff/" + fresh.Source.SHA256
	if _, err := fresh.StartBuild("bld_rec2", now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := fresh.MarkBuildReady("bld_rec2", []deployment.Artifact{ready},
		now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := deployments.Save(ctx, fresh); err != nil {
		t.Fatal(err)
	}

	running, err := execution.New("exe_running", project.ID, fresh.ID, "bld_rec2",
		json.RawMessage(`1`), now.Add(3*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := executions.Create(ctx, running); err != nil {
		t.Fatal(err)
	}
	if _, err := executions.ClaimQueued(ctx, now.Add(4*time.Second)); err != nil {
		t.Fatal(err)
	}
	stillQueued, err := execution.New("exe_queued", project.ID, fresh.ID, "bld_rec2",
		json.RawMessage(`2`), now.Add(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	if err := executions.Create(ctx, stillQueued); err != nil {
		t.Fatal(err)
	}

	count, err := executions.RecoverRunning(ctx, now.Add(6*time.Second),
		execution.Failure{Code: "worker_restarted", Message: "restarted"})
	if err != nil || count != 1 {
		t.Fatalf("recover executions = %d, %v", count, err)
	}
	failed, err := executions.GetByID(ctx, organizationID, "exe_running")
	if err != nil || failed.Status != execution.StatusFailed || failed.Failure == nil {
		t.Fatalf("recovered execution = %#v, %v", failed, err)
	}
	untouched, err := executions.GetByID(ctx, organizationID, "exe_queued")
	if err != nil || untouched.Status != execution.StatusQueued {
		t.Fatalf("queued execution changed = %#v, %v", untouched, err)
	}
}
