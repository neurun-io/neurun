CREATE TABLE api_keys (
    id          text PRIMARY KEY,
    project_id  text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     text,
    name        text NOT NULL,
    key_prefix  text NOT NULL UNIQUE,
    key_hash    bytea NOT NULL,
    scopes      text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    CONSTRAINT api_keys_hash_length CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 128),
    FOREIGN KEY (user_id, project_id)
        REFERENCES users(id, project_id) ON DELETE CASCADE
);

CREATE INDEX api_keys_project_created
    ON api_keys(project_id, created_at DESC, id DESC);
