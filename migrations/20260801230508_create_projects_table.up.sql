CREATE TABLE projects (
    id          text PRIMARY KEY,
    name        text NOT NULL UNIQUE,
    created_at  timestamptz NOT NULL,
    updated_at  timestamptz NOT NULL,
    CONSTRAINT projects_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);
