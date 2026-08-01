CREATE TABLE builds (
    id             text PRIMARY KEY,
    deployment_id  text NOT NULL REFERENCES deployments(id) ON DELETE CASCADE,
    number         integer NOT NULL,
    status         text NOT NULL,
    runtime        text NOT NULL,
    entrypoint     text NOT NULL,
    source_sha256  text NOT NULL,
    artifacts      jsonb NOT NULL DEFAULT '[]'::jsonb,
    failure        jsonb,
    started_at     timestamptz NOT NULL,
    finished_at    timestamptz,
    version        bigint NOT NULL DEFAULT 1 CHECK (version > 0),
    UNIQUE (deployment_id, number),
    UNIQUE (id, deployment_id),
    CONSTRAINT builds_runtime_python CHECK (runtime = 'python'),
    CONSTRAINT builds_status_known CHECK (
        status IN ('uploaded', 'building', 'ready', 'failed')
    ),
    CONSTRAINT builds_source_sha256_length CHECK (length(source_sha256) = 64)
);

CREATE INDEX builds_deployment_number
    ON builds(deployment_id, number DESC);
