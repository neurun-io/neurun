-- A key carries scopes, not a project. Scopes are the authorization model; a
-- project column would cap a key at one project for no reason and duplicate the
-- decision scopes already make.
CREATE TABLE api_keys (
    id          text PRIMARY KEY,
    -- SET NULL, not CASCADE: removing the person who minted a key must not
    -- remove the key, and nothing a user owns disappears with them. Revoking a
    -- key stays a separate, deliberate act.
    user_id     text REFERENCES users(id) ON DELETE SET NULL,
    name        text NOT NULL,
    key_prefix  text NOT NULL UNIQUE,
    key_hash    bytea NOT NULL,
    scopes      text[] NOT NULL DEFAULT '{}',
    created_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    CONSTRAINT api_keys_hash_length CHECK (octet_length(key_hash) = 32),
    CONSTRAINT api_keys_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 128)
);

CREATE INDEX api_keys_created
    ON api_keys(created_at DESC, id DESC);
