CREATE TABLE executions (
    id                     text PRIMARY KEY,
    deployment_id          text NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    build_id               text NOT NULL REFERENCES builds(id),
    status                 text NOT NULL,
    input                  jsonb NOT NULL,
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
    CONSTRAINT executions_logs_bounded CHECK (octet_length(logs) <= 262144)
);

CREATE INDEX executions_deployment_created
    ON executions(deployment_id, created_at DESC, id DESC);
CREATE INDEX executions_queue
    ON executions(status, created_at, id);
