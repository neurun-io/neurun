-- A build is a runnable app: whose it is, the layers it is made of, each named
-- for what it is to the runtime, and enough about them to know what runs them.
-- It carries no status and no failure — those belong to the deployment that
-- ran it — and it is never rewritten, which is why it has no version column.
CREATE TABLE builds (
    id             text PRIMARY KEY,
    app_id         text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    deployment_id  text NOT NULL UNIQUE REFERENCES deployments(id) ON DELETE CASCADE,
    runtime        text NOT NULL,
    source_sha256  text NOT NULL,
    artifacts      jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at     timestamptz NOT NULL,
    CONSTRAINT builds_runtime_known CHECK (runtime IN ('python', 'rust', 'go', 'ruby', 'node')),
    CONSTRAINT builds_source_sha256_length CHECK (length(source_sha256) = 64)
);

CREATE INDEX builds_app_created ON builds(app_id, created_at DESC, id DESC);
