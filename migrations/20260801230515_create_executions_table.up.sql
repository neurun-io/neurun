CREATE TABLE executions (
    id                     text PRIMARY KEY,
    project_id             text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    deployment_id          text NOT NULL,
    build_id               text NOT NULL,
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
    CONSTRAINT executions_logs_bounded CHECK (octet_length(logs) <= 262144),
    FOREIGN KEY (deployment_id, project_id)
        REFERENCES deployments(id, project_id) ON DELETE CASCADE,
    FOREIGN KEY (build_id, deployment_id)
        REFERENCES builds(id, deployment_id)
);

CREATE INDEX executions_project_deployment_created
    ON executions(project_id, deployment_id, created_at DESC, id DESC);
CREATE INDEX executions_queue
    ON executions(status, created_at, id);
