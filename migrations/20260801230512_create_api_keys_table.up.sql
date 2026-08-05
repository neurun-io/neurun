CREATE TABLE api_keys (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    -- SET NULL, not CASCADE: deleting the person who minted a key must not
    -- delete the key.
    user_id         text REFERENCES users(id) ON DELETE SET NULL,
    name            text NOT NULL,
    key_prefix      text NOT NULL UNIQUE,
    key_hash        bytea NOT NULL,
    scopes          text[] NOT NULL DEFAULT '{}',
    created_at      timestamptz NOT NULL,
    revoked_at      timestamptz,
    CONSTRAINT api_keys_hash_length CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 128)
);

CREATE INDEX api_keys_created
    ON api_keys(organization_id, created_at DESC, id DESC);
