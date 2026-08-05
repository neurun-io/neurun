CREATE TABLE organization_members (
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id         text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    role            text NOT NULL,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    PRIMARY KEY (organization_id, user_id),
    CONSTRAINT organization_members_role_known CHECK (
        role IN ('admin', 'operator', 'viewer')
    )
);

CREATE INDEX organization_members_user ON organization_members(user_id);
