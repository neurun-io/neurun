# User application CI/CD template

This template keeps user code in the user's repository and CI account. Neurun
validates declarative JSON and provides the API used by smoke tests; the Neurun
server never clones, builds, deploys, or runs application source.

## Install

1. Copy this directory's `.github` and `neurun` folders into the application
   repository.
2. Replace the reusable-workflow placeholder with a full 40-character
   `neurun-io` commit SHA. Set `contracts-ref` to that same SHA.
3. Replace the Neurun image placeholder with a published image reference pinned
   by `@sha256:` digest. Tags such as `latest` are rejected.
4. Set the lowercase `image-name`, then edit the two JSON descriptors.
5. Add the required Bash scripts below and protect the `production` and
   `release` GitHub Environments with reviewers or deployment rules.

The application repository must also contain a Dockerfile at the configured
path. The application descriptor is CI metadata and is not uploaded to the
Neurun server.

The workflow deliberately fails until its required scripts exist:

- `scripts/ci/test` installs pinned dependencies and runs the application's
  actual checks.
- `scripts/ci/build` creates the configured `dist` directory and its releasable
  files.
- `scripts/ci/smoke` exercises the built application against
  `$NEURUN_BASE_URL`. It can use `$NEURUN_API_KEY`, `$USER_APP_IMAGE`, and
  `$NEURUN_WORKFLOW_PATH`; it must return non-zero on failure.
- `scripts/ci/deploy` deploys the exact `$USER_APP_IMAGE` (which includes a
  digest), not a mutable tag. It may also read `$USER_APP_DIGEST` and
  `$USER_APP_ARTIFACT_DIRECTORY`.

Each script is invoked with `bash`, so executable bits are optional. Keep
credentials out of scripts, JSON, Docker build arguments, artifacts, logs, and
container layers.

## Secrets and permissions

The default pinned-image smoke job receives no repository secret. The local
Neurun API key is ephemeral and non-sensitive. To smoke test an existing
endpoint instead, select `smoke-mode: endpoint`, use an HTTPS URL, and pass
`NEURUN_API_KEY`; endpoint mode is rejected on pull requests.

`DEPLOYMENT_TOKEN` is optional and is read directly from the application's
protected `production` Environment by the caller workflow; reusable workflows
do not receive environment secrets. Prefer short-lived cloud credentials
obtained with GitHub OIDC and remove the token reference when it is not needed.
A private contracts repository may receive a read-only `CONTRACTS_TOKEN`;
public repositories need none. The custom contracts token is never selected on
a pull request. Registry credentials default to the run's scoped GitHub token;
map `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` explicitly for another registry.

When one of those optional reusable-workflow secrets is needed, add only that
name beneath the `pipeline` job:

```yaml
    secrets:
      NEURUN_API_KEY: ${{ secrets.NEURUN_API_KEY }}
      CONTRACTS_TOKEN: ${{ secrets.CONTRACTS_TOKEN }}
      REGISTRY_USERNAME: ${{ secrets.REGISTRY_USERNAME }}
      REGISTRY_PASSWORD: ${{ secrets.REGISTRY_PASSWORD }}
```

Pull requests receive read-only jobs and no deployment credentials. Publication
uses the source commit as the tag, records OCI revision/source labels, attaches
BuildKit provenance and SBOM data, publishes an SPDX SBOM artifact, and emits
GitHub attestations. Deployment consumes the returned digest-addressed image.

Do not use `secrets: inherit`, unpinned Neurun images, floating reusable-workflow
refs, or deployment scripts from untrusted branches.
