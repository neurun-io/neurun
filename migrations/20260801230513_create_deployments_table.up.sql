-- A deployment is the act of turning one commit into a build: how far it got
-- and what the toolchain printed on the way. What it produced is the build that
-- names it, and a deployment that failed before the toolchain ran produced
-- none. The source it built is not kept: commit_sha is where those bytes are.
CREATE TABLE deployments (
    id          text PRIMARY KEY,
    app_id      text NOT NULL REFERENCES apps(id) ON DELETE CASCADE,
    runtime     text NOT NULL,
    status      text NOT NULL,
    commit_sha  text,
    git_ref     text,
    failure     jsonb,
    logs        text NOT NULL DEFAULT '',
    started_at  timestamptz,
    finished_at timestamptz,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    version     bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT deployments_runtime_known CHECK (runtime IN ('python', 'rust', 'go', 'ruby', 'node')),
    CONSTRAINT deployments_status_known CHECK (
        status IN ('queued', 'building', 'publishing', 'ready', 'failed')
    ),
    CONSTRAINT deployments_logs_bounded CHECK (octet_length(logs) <= 262144),
    CONSTRAINT deployments_commit_sha_length CHECK (
        commit_sha IS NULL OR length(commit_sha) = 40
    ),
    CONSTRAINT deployments_git_ref_needs_commit CHECK (
        git_ref IS NULL OR commit_sha IS NOT NULL
    )
);

CREATE INDEX deployments_app_created
    ON deployments(app_id, created_at DESC, id DESC);
