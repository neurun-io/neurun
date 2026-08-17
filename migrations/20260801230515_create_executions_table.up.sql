-- An execution runs one of an app's builds. How that build came to exist is
-- the build's own business.
CREATE TABLE executions (
    id                     text PRIMARY KEY,
    app_id                 text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    build_id               text NOT NULL REFERENCES builds(id) ON DELETE CASCADE,
    status                 text NOT NULL,
    input                  jsonb NOT NULL,
    -- What the app returned, while it is small enough to belong on the row.
    -- Anything larger belongs in the artifact store, addressed from here.
    output                 jsonb,
    failure                jsonb,
    logs                   text NOT NULL DEFAULT '',
    created_at             timestamptz NOT NULL,
    started_at             timestamptz,
    finished_at            timestamptz,
    rerun_of_execution_id  text REFERENCES executions(id) ON DELETE SET NULL,
    version                bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT executions_status_known CHECK (
        status IN ('queued', 'running', 'succeeded', 'failed')
    ),
    CONSTRAINT executions_logs_bounded CHECK (octet_length(logs) <= 262144),
    CONSTRAINT executions_output_bounded CHECK (
        output IS NULL OR pg_column_size(output) <= 4194304
    )
);

CREATE INDEX executions_app_created
    ON executions(app_id, created_at DESC, id DESC);
CREATE INDEX executions_build_created
    ON executions(build_id, created_at DESC, id DESC);
CREATE INDEX executions_queue
    ON executions(status, created_at, id);
