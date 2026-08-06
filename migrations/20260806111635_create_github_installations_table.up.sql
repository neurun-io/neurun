CREATE TABLE github_installations (
    id              text PRIMARY KEY,
    organization_id text NOT NULL UNIQUE REFERENCES organizations(id) ON DELETE CASCADE,
    installation_id bigint NOT NULL UNIQUE,
    account_login   text NOT NULL,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL
);
