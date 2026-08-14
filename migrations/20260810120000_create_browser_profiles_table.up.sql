CREATE TABLE browser_profiles (
    id              text PRIMARY KEY,
    organization_id text NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name            text NOT NULL,
    -- Every profile wears one: a browser presenting as itself is the easiest
    -- kind to catch, so a profile created without an identity is given one.
    -- It names its own browser, which is why there is no column for that.
    identity        jsonb NOT NULL,
    cookies         jsonb NOT NULL DEFAULT '[]'::jsonb,
    local_storage   jsonb NOT NULL DEFAULT '{}'::jsonb,
    session_storage jsonb NOT NULL DEFAULT '{}'::jsonb,
    created_at      timestamptz NOT NULL,
    updated_at      timestamptz NOT NULL,
    UNIQUE (organization_id, name),
    CONSTRAINT browser_profiles_name_nonblank CHECK (length(btrim(name)) BETWEEN 1 AND 120),
    CONSTRAINT browser_profiles_identity_object CHECK (
        jsonb_typeof(identity) = 'object'
    ),
    CONSTRAINT browser_profiles_browser_known CHECK (
        identity->>'browser' IN ('chrome', 'safari')
    ),
    CONSTRAINT browser_profiles_cookies_array CHECK (jsonb_typeof(cookies) = 'array'),
    CONSTRAINT browser_profiles_local_storage_object CHECK (
        jsonb_typeof(local_storage) = 'object'
    ),
    CONSTRAINT browser_profiles_session_storage_object CHECK (
        jsonb_typeof(session_storage) = 'object'
    )
);

CREATE INDEX browser_profiles_organization_created
    ON browser_profiles(organization_id, created_at DESC, id DESC);
