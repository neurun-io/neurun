-- owner_user_id is UNIQUE: a user owns at most one organization, though they
-- may be a member of any number.
CREATE TABLE organizations (
    id            text PRIMARY KEY,
    owner_user_id text NOT NULL UNIQUE REFERENCES users(id) ON DELETE RESTRICT,
    name          text NOT NULL,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT organizations_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120)
);
