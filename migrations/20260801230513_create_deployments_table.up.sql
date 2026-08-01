CREATE TABLE deployments (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    app_id      text NOT NULL,
    runtime     text NOT NULL,
    entrypoint  text NOT NULL,
    status      text NOT NULL,
    source      jsonb NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    version     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (id, project_id),
    CONSTRAINT deployments_runtime_python CHECK (runtime = 'python'),
    CONSTRAINT deployments_status_known CHECK (
        status IN ('uploaded', 'building', 'ready', 'failed')
    ),
    FOREIGN KEY (app_id, project_id)
        REFERENCES apps(id, project_id) ON DELETE CASCADE
);

CREATE INDEX deployments_project_created
    ON deployments(project_id, created_at DESC, id DESC);
CREATE INDEX deployments_app_created
    ON deployments(project_id, app_id, created_at DESC, id DESC);
