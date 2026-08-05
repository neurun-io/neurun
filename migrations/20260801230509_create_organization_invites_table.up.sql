CREATE TABLE organization_invites (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    email           text NOT NULL,
    role            text NOT NULL,
    token_hash      bytea NOT NULL UNIQUE,
    invited_by      text REFERENCES users(id) ON DELETE SET NULL,
    created_at      timestamptz NOT NULL,
    expires_at      timestamptz NOT NULL,
    accepted_at     timestamptz,
    revoked_at      timestamptz,
    CONSTRAINT organization_invites_role_known CHECK (
        role IN ('admin', 'operator', 'viewer')
    ),
    CONSTRAINT organization_invites_email_valid CHECK (
        email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 320
    ),
    CONSTRAINT organization_invites_token_hash_length CHECK (
        octet_length(token_hash) = 32
    )
);

-- One live invite per address per organization; spent ones may repeat.
CREATE UNIQUE INDEX organization_invites_pending
    ON organization_invites(organization_id, email)
    WHERE accepted_at IS NULL AND revoked_at IS NULL;

CREATE INDEX organization_invites_listing
    ON organization_invites(organization_id, created_at DESC, id DESC);
