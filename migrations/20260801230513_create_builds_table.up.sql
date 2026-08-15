-- A build is what a deployment produced: the layers it made, each named for
-- what it is to the runtime, and enough about them to know what runs them. It
-- carries no status and no failure — those belong to the deployment that ran
-- it — and it is never rewritten, which is why it has no version column.
CREATE TABLE builds (
    id             text PRIMARY KEY,
    runtime        text NOT NULL,
    entrypoint     text NOT NULL,
    source_sha256  text NOT NULL,
    artifacts      jsonb NOT NULL DEFAULT '[]'::jsonb,
    created_at     timestamptz NOT NULL,
    CONSTRAINT builds_runtime_known CHECK (runtime IN ('python', 'rust', 'go', 'ruby', 'node')),
    CONSTRAINT builds_source_sha256_length CHECK (length(source_sha256) = 64)
);

CREATE INDEX builds_created ON builds(created_at DESC, id DESC);
