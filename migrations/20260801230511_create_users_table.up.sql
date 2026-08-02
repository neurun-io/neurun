-- A user is a global identity. Which projects they may act in, and what they
-- may do there, lives in memberships -- so deleting a project never deletes a
-- person, and one account can hold a different role in each project.
CREATE TABLE users (
    id            text PRIMARY KEY,
    username      text NOT NULL UNIQUE,
    display_name  text NOT NULL,
    password_hash text NOT NULL,
    disabled      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT users_username_valid CHECK (
        username = lower(btrim(username)) AND length(username) BETWEEN 1 AND 64
    ),
    CONSTRAINT users_display_name_nonblank CHECK (
        length(btrim(display_name)) BETWEEN 1 AND 128
    ),
    CONSTRAINT users_password_hash_encoded CHECK (
        password_hash LIKE '$2%' AND length(password_hash) = 60
    )
);
