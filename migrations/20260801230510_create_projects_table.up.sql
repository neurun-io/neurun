CREATE TABLE projects (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (organization_id, name),
    CONSTRAINT projects_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);

CREATE INDEX projects_organization
    ON projects(organization_id, created_at DESC, id DESC);
