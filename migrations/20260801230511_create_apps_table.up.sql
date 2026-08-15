CREATE TABLE apps (
    id              text PRIMARY KEY,
    project_id      text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name            text NOT NULL,
    repository      text,
    production_ref  text,
    active_build_id text,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (project_id, name),
    CONSTRAINT apps_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT apps_repository_shape CHECK (
        repository IS NULL OR repository ~ '^[^/[:space:]]+/[^/[:space:]]+$'
    ),
    CONSTRAINT apps_production_ref_needs_repository CHECK (
        production_ref IS NULL OR repository IS NOT NULL
    )
);
