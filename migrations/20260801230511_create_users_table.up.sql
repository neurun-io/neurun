CREATE TABLE users (
    id            text PRIMARY KEY,
    project_id    text NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    username      text NOT NULL,
    display_name  text NOT NULL,
    role          text NOT NULL,
    password_hash text NOT NULL,
    disabled      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    UNIQUE (username),
    UNIQUE (id, project_id),
    CONSTRAINT users_username_valid CHECK (
        username = lower(btrim(username)) AND length(username) BETWEEN 1 AND 64
    ),
    CONSTRAINT users_display_name_nonblank CHECK (
        length(btrim(display_name)) BETWEEN 1 AND 128
    ),
    CONSTRAINT users_role_known CHECK (role IN ('admin', 'operator', 'viewer')),
    CONSTRAINT users_password_hash_encoded CHECK (
        password_hash LIKE '$2%' AND length(password_hash) = 60
    )
);

CREATE INDEX users_project_created
    ON users(project_id, created_at DESC, id DESC);
