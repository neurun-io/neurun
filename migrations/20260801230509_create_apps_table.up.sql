CREATE TABLE apps (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    name        text NOT NULL,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    UNIQUE (project_id, name),
    UNIQUE (id, project_id),
    CONSTRAINT apps_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);
