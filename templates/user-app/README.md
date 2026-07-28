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
4. Set the lowercase `image-name`, then edit the two JSON descriptors. The
   workflow, container, and artifact paths in `neurun/application.json` must
   exactly match the reusable-workflow inputs; CI rejects drift.
5. Add the required Bash scripts below.
6. Protect the default branch and release-tag pattern (for example `v*`) with
   branch protection or repository rulesets. Publication, endpoint smoke
   checks, deployment, and releases require GitHub to report the selected ref
   as protected. A manual run from an arbitrary feature branch cannot receive
   the endpoint key or publish an image.
7. Protect the `production` and `release` GitHub Environments with reviewers,
   allowed-branch rules, or deployment protection rules.

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
public repositories need none. The custom contracts token is selected only on
a protected default branch or protected tag, never on a pull request or an
arbitrary manual ref. Registry credentials default to the run's scoped GitHub
token; map `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` explicitly for another
registry.

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
uses the source commit as the tag and records OCI revision/source labels. CI
smoke-tests one local image, exports that exact image with its SHA-256 checksum
and image ID, then reloads and stages those same bytes without rebuilding.
Only after provenance and SBOM attestations plus the release SBOM artifact
succeed does CI promote that digest to the commit-SHA release tag. Deployment
consumes the returned digest-addressed image. Artifact names include the run
attempt so GitHub's immutable artifact store also supports workflow reruns. On
a rerun, CI reuses an existing commit-SHA tag only when it already names the
same digest; a different digest fails closed instead of rewriting that tag.

`neurun/application.json` is enforced CI metadata rather than documentation
only. Its workflow, container context, Dockerfile, and artifact directory must
match the caller inputs. The repository-owned validator also rejects duplicate
workflow step IDs, unknown dependencies, and dependency cycles. That validator
comes from the same pinned Neurun contracts revision and uses only the Python
standard library; validation does not download executable npm packages.

## Platform limitations

GitHub artifact attestations are available for public repositories on current
GitHub plans. Private and internal repositories require GitHub Enterprise
Cloud. Because publication treats provenance and SBOM attestations as required,
the publish job fails on an unsupported private-repository plan before creating
the documented commit-SHA release tag. A failed publish may leave a
`ci-<run>-<attempt>` staging tag for registry retention cleanup, but callers and
deployment outputs never treat that tag as a release.

A `CONTRACTS_TOKEN` can read a separate private contracts repository on trusted
protected default-branch or tag events. It is deliberately not exposed to
pull-request or arbitrary manual-ref workflows.
GitHub's run token is scoped to the application repository, so pull requests
cannot check out a different private contracts repository. Use the public
released Neurun contracts, vendor the pinned contract files, or accept that
private cross-repository contract validation starts only after merge.

Do not use `secrets: inherit`, unpinned Neurun images, floating reusable-workflow
refs, or deployment scripts from untrusted branches.
