CREATE TABLE deployments (
    id          text PRIMARY KEY,
    app_id      text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    runtime     text NOT NULL,
    entrypoint  text NOT NULL,
    status      text NOT NULL,
    source      jsonb NOT NULL,
    commit_sha  text,
    git_ref     text,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    version     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT deployments_runtime_python CHECK (runtime = 'python'),
    CONSTRAINT deployments_status_known CHECK (
        status IN ('uploaded', 'building', 'ready', 'failed')
    ),
    CONSTRAINT deployments_commit_sha_length CHECK (
        commit_sha IS NULL OR length(commit_sha) = 40
    ),
    CONSTRAINT deployments_git_ref_needs_commit CHECK (
        git_ref IS NULL OR commit_sha IS NOT NULL
    )
);

CREATE INDEX deployments_app_created
    ON deployments(app_id, created_at DESC, id DESC);
