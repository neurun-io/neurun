CREATE TABLE users (
    id            text PRIMARY KEY,
    email         text NOT NULL UNIQUE,
    password_hash text NOT NULL,
    disabled      boolean NOT NULL DEFAULT false,
    created_at    timestamptz NOT NULL,
    updated_at    timestamptz NOT NULL,
    CONSTRAINT users_email_valid CHECK (
        email = lower(btrim(email)) AND length(email) BETWEEN 3 AND 320
    ),
    CONSTRAINT users_password_hash_encoded CHECK (
        password_hash LIKE '$2%' AND length(password_hash) = 60
    )
);
