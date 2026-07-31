BEGIN;

CREATE TABLE projects (
    id          text PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT projects_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);

CREATE TABLE apps (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id),
    name        text NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (project_id, name),
    UNIQUE (id, project_id),
    CONSTRAINT apps_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);

CREATE TABLE users (
    id            text PRIMARY KEY,
    project_id    text NOT NULL REFERENCES projects(id),
    username      text NOT NULL,
    display_name  text NOT NULL,
    role          text NOT NULL,
    password_hash text NOT NULL,
    disabled      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    UNIQUE (username),
    UNIQUE (id, project_id),
    CONSTRAINT users_username_valid CHECK (
        username = lower(btrim(username)) AND length(username) BETWEEN 1 AND 64
    ),
    CONSTRAINT users_display_name_nonblank CHECK (
        length(btrim(display_name)) BETWEEN 1 AND 128
    ),
    CONSTRAINT users_role_known CHECK (role IN ('admin', 'operator', 'viewer')),
    CONSTRAINT users_password_hash_encoded CHECK (password_hash LIKE 'pbkdf2-sha256$%')
);

CREATE TABLE api_keys (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id),
    user_id     text,
    name        text NOT NULL,
    key_prefix  text NOT NULL UNIQUE,
    key_hash    bytea NOT NULL,
    scopes      text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    CONSTRAINT api_keys_hash_length CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    FOREIGN KEY (user_id, project_id) REFERENCES users(id, project_id)
);

CREATE TABLE deployments (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id),
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
    FOREIGN KEY (app_id, project_id) REFERENCES apps(id, project_id)
);

CREATE TABLE builds (
    id            text PRIMARY KEY,
    deployment_id text NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    number         integer NOT NULL,
    status         text NOT NULL,
    runtime        text NOT NULL,
    entrypoint     text NOT NULL,
    source_sha256  text NOT NULL,
    artifacts      jsonb NOT NULL DEFAULT '[]'::jsonb,
    failure        jsonb,
    started_at     timestamptz NOT NULL,
    finished_at    timestamptz,
    version        bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (deployment_id, number),
    UNIQUE (id, deployment_id),
    CONSTRAINT builds_runtime_python CHECK (runtime = 'python'),
    CONSTRAINT builds_status_known CHECK (
        status IN ('uploaded', 'building', 'ready', 'failed')
    ),
    CONSTRAINT builds_source_sha256_length CHECK (length(source_sha256) = 64)
);

CREATE TABLE executions (
    id                        text PRIMARY KEY,
    project_id                text NOT NULL REFERENCES projects(id),
    deployment_id             text NOT NULL,
    build_id                  text NOT NULL,
    status                    text NOT NULL,
    input                     jsonb NOT NULL,
    output                    jsonb,
    failure                   jsonb,
    logs                      text NOT NULL DEFAULT '',
    created_at                timestamptz NOT NULL,
    started_at                timestamptz,
    finished_at               timestamptz,
    rerun_of_execution_id     text REFERENCES executions(id),
    version                   bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    CONSTRAINT executions_status_known CHECK (
        status IN ('queued', 'running', 'succeeded', 'failed')
    ),
    CONSTRAINT executions_logs_bounded CHECK (octet_length(logs) <= 262144),
    FOREIGN KEY (deployment_id, project_id)
        REFERENCES deployments(id, project_id) ON DELETE CASCADE,
    FOREIGN KEY (build_id, deployment_id)
        REFERENCES builds(id, deployment_id)
);

CREATE INDEX deployments_project_created
    ON deployments(project_id, created_at DESC, id DESC);
CREATE INDEX deployments_app_created
    ON deployments(project_id, app_id, created_at DESC, id DESC);
CREATE INDEX builds_deployment_number
    ON builds(deployment_id, number DESC);
CREATE INDEX executions_project_deployment_created
    ON executions(project_id, deployment_id, created_at DESC, id DESC);
CREATE INDEX executions_queue
    ON executions(status, created_at, id);
CREATE INDEX users_project_created
    ON users(project_id, created_at DESC, id DESC);
CREATE INDEX api_keys_project_created
    ON api_keys(project_id, created_at DESC, id DESC);

COMMIT;
