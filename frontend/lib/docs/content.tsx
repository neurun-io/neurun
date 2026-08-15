import type { ReactNode } from "react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Callout } from "@/components/neurun/feedback";
import { Panel } from "@/components/neurun/panel";
import { C, H2, P, Rows, Snippet, Split } from "@/components/docs/prose";

/**
 * The documentation collection.
 *
 * The tree, the frontmatter and the table of contents come from this model —
 * the renderer holds none of it. Every endpoint, field and status here is taken
 * from `api/openapi.yaml` and the concept notes in `docs/`; a docs page that
 * drifts from the contract is a support ticket with extra steps.
 */

export interface DocsPage {
  slug: string;
  label: string;
  group: string;
  title: string;
  lead: ReactNode;
  tags: string[];
  source: string;
  toc: { id: string; label: string }[];
  body: ReactNode;
}

export const DOCS_NAV = [
  { label: "Start", items: [{ slug: "quickstart", label: "Quickstart", icon: "play" }] },
  {
    label: "Deploy",
    items: [
      { slug: "apps", label: "Apps and deployments", icon: "box" },
      { slug: "builds", label: "Builds", icon: "list-checks" },
    ],
  },
  {
    label: "Execute",
    items: [
      { slug: "executions", label: "Executions", icon: "activity" },
      { slug: "servers", label: "Servers", icon: "server-cog" },
    ],
  },
  {
    label: "Contracts",
    items: [
      { slug: "authentication", label: "Authentication", icon: "key-round" },
      { slug: "errors", label: "Error model", icon: "triangle-alert" },
    ],
  },
];

export const DOCS_ORDER = [
  "quickstart",
  "apps",
  "builds",
  "executions",
  "servers",
  "authentication",
  "errors",
];

export const DOCS: Record<string, DocsPage> = {
  quickstart: {
    slug: "quickstart",
    label: "Quickstart",
    group: "Start",
    title: "Quickstart",
    lead: "Issue a key, create an app, connect it to a repository, and run it. Ten minutes, and no infrastructure of your own.",
    tags: ["v1", "0.1.0", "free credit"],
    source: "content/collections/neurun/quickstart.mdx",
    toc: [
      { id: "reach", label: "Reach the control plane" },
      { id: "account", label: "Create an account" },
      { id: "key", label: "Issue a key" },
      { id: "app", label: "Create an app" },
      { id: "deploy", label: "Connect it to a repository" },
      { id: "run", label: "Run it" },
    ],
    body: (
      <>
        <H2 id="reach">Reach the control plane</H2>
        <P>
          Two endpoints need no credential: <C>/readyz</C> tells you the plane is serving, and{" "}
          <C>/version</C> tells you which build answered. Check both before you debug anything else.
        </P>
        <Snippet filename="shell">{`export NEURUN_URL=https://api.neurun.dev

curl -sS $NEURUN_URL/readyz
curl -sS $NEURUN_URL/version
# {"version":"0.1.0","commit":"9f3a41c","api_version":"v1",
#  "schema_version":"12","built_at":"2026-01-14T09:12:03Z"}`}</Snippet>

        <H2 id="account">Create an account</H2>
        <P>
          Registration is the only way an account comes into being — there is no CLI to run on the
          host. It creates the account as an administrator, creates the project it names, and signs
          you in by setting the session cookie, so one request takes a fresh install to a usable
          dashboard.
        </P>
        <Snippet filename="shell">{`curl -sS -X POST $NEURUN_URL/v1/auth/register \\
  -c session.txt \\
  -d '{"username":"ada",
       "password":"a-long-password"}'

# 201 {"user":{...},"dashboard":{...}}`}</Snippet>

        <H2 id="key">Issue a key</H2>
        <P>
          A session is for a person at a browser; a program gets a key. Create one with the scopes
          the work actually needs — the secret is returned exactly once, only its SHA-256 digest is
          stored, and the plaintext is unrecoverable.
        </P>
        <Snippet filename="shell">{`curl -sS -X POST $NEURUN_URL/v1/api-keys \\
  -b session.txt \\
  -d '{"name":"ci","scopes":["apps:write","deployments:write","executions:write"]}'

export NEURUN_KEY=neu_live_a41f.•••   # shown once, never again`}</Snippet>
        <Callout kind="note" title="A key carries scopes, not a project">
          Scopes are the entire authorization model, so a key is not pinned to a project. Projects
          scope <em>resources</em>, not callers. A key can never be granted a scope its creator does
          not already hold, so a limited key cannot mint an unlimited one.
        </Callout>

        <H2 id="app">Create an app</H2>
        <P>
          An app is the thing you deploy to, and it must exist first. Nothing auto-creates one:{" "}
          a deploy looks up the <C>app_id</C> and fails with <C>app not found</C> when it is
          missing. That is deliberate — auto-creation means a typo in a client silently
          produces a second app that looks fine and receives none of your traffic.
        </P>
        <Snippet filename="shell">{`curl -sS -X POST $NEURUN_URL/v1/apps \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"project_id":"prj_4T1M0","name":"pricing-crawler"}'

# {"id":"app_7QK2M0X4","project_id":"prj_4T1M0",
#  "name":"pricing-crawler", ...}`}</Snippet>

        <H2 id="deploy">Connect it to a repository</H2>
        <P>
          Source is never uploaded. Point the app at a repository the GitHub App is installed on, and
          every push to its production ref fetches that commit and builds it. Deploying a ref by hand
          takes the same path, so a manual deploy and a pushed one produce the same kind of record.
        </P>
        <Snippet filename="shell">{`curl -sS -X PUT $NEURUN_URL/v1/apps/app_7QK2M0X4/repository \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"repository":"acme/pricing-crawler","production_ref":"main"}'

# then push to main — or deploy a ref yourself
curl -sS -X POST $NEURUN_URL/v1/github/deployments \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"app_id":"app_7QK2M0X4","ref":"main"}'

HTTP/1.1 201 Created
# {"id":"dep_01HXQ8F2K9","status":"ready",
#  "build":{"id":"bld_9F3AC41","runtime":"python"}}`}</Snippet>

        <H2 id="run">Run it</H2>
        <P>
          Creating an execution returns <C>202</C> immediately and pins it to the latest ready build.
          Poll the execution until it goes terminal; only then are <C>output</C>, <C>logs</C> and{" "}
          <C>failure</C> settled.
        </P>
        <Split>
          <Panel flush label="Create" actions={<Meta>202</Meta>}>
            <Snippet>{`POST /v1/deployments/
     dep_01HXQ8F2K9/executions

{"input": {"url": "https://example.com"}}`}</Snippet>
          </Panel>
          <Panel flush label="Accepted" actions={<Meta>queued</Meta>}>
            <Snippet>{`{
  "id": "exe_01HXQ8F2M4",
  "status": "queued",
  "build_id": "bld_9F3AC41",
  "input": {"url": "https://example.com"}
}`}</Snippet>
          </Panel>
        </Split>
        <P>
          The <C>build_id</C> is the point. You deployed source; the server tells you exactly which
          immutable build will answer, and keeps naming it long after you have rebuilt.
        </P>
      </>
    ),
  },

  apps: {
    slug: "apps",
    label: "Apps and deployments",
    group: "Deploy",
    title: "Apps and deployments",
    lead: "An app is a named thing you deploy to. A deployment is one commit of its repository, plus every build made from that commit.",
    tags: ["v1", "stable"],
    source: "content/collections/neurun/apps.mdx",
    toc: [
      { id: "app", label: "The app must exist first" },
      { id: "source", label: "One deployment, one build" },
      { id: "entrypoint", label: "Runtime and entrypoint" },
      { id: "status", label: "Status is the deployment's own" },
      { id: "delete", label: "Deleting cascades" },
    ],
    body: (
      <>
        <H2 id="app">The app must exist first</H2>
        <P>
          An SDK cannot create an app by deploying to it. Create apps explicitly with{" "}
          <C>POST /v1/apps</C>, then connect each one to a repository. The app is also what decides a deployment&apos;s
          project — the caller never supplies a project when deploying.
        </P>
        <Rows
          items={[
            { term: "id", body: "Server-issued, prefixed app_." },
            { term: "project_id", body: "The owning project. Set at creation and never moved." },
            { term: "name", body: "1–120 characters, unique within the project." },
            { term: "created_at", body: "Both timestamps are RFC 3339 with an explicit offset." },
          ]}
        />

        <H2 id="source">One deployment, one build</H2>
        <P>
          A deployment is one attempt at one commit, and a build is what came out of it. An attempt
          that failed has a <C>failure</C> and no build at all — deploy the commit again and you get
          a second deployment, not a second build under the first.
        </P>
        <Snippet filename="shell">{`# every deployment for one app, newest first
curl -sS "$NEURUN_URL/v1/deployments?app_id=app_7QK2M0X4&limit=20" \\
  -H "authorization: Bearer $NEURUN_KEY"`}</Snippet>

        <H2 id="entrypoint">Runtime and entrypoint</H2>
        <P>
          <C>runtime</C> is <C>python</C>. It is a constant in the contract today rather than an open
          enum, so treat an unrecognised value as a server you do not understand rather than a
          runtime you can guess at. <C>entrypoint</C> defaults to <C>main.py:handler</C> and is
          normalized before storage.
        </P>

        <H2 id="status">Status mirrors the newest build</H2>
        <P>
          A deployment&apos;s <C>status</C> always reflects its newest build — <C>uploaded</C> before
          any build exists, then <C>building</C>, <C>ready</C> or <C>failed</C>. The invariant is
          enforced on every write, so you never have to reconcile the two yourself.
        </P>
        <Rows
          items={[
            { term: "uploaded", body: "Source fetched and stored, no build started yet." },
            { term: "building", body: "The newest build is running." },
            { term: "ready", body: "The newest build produced artifacts. Executions may pin to it." },
            { term: "failed", body: "The newest build carries a failure code and message." },
          ]}
        />

        <H2 id="delete">Deleting cascades</H2>
        <Callout kind="warning" title="Deleting an app deletes its history">
          <C>DELETE /v1/apps/{"{app_id}"}</C> cascades to the app&apos;s deployments, builds and
          executions, and requires the app&apos;s exact name in a <C>confirm</C> query parameter. The
          dashboard asks you to type the name for the same reason: there is no undo, and the
          executions it removes are the record of what ran.
        </Callout>
      </>
    ),
  },

  builds: {
    slug: "builds",
    label: "Builds",
    group: "Deploy",
    title: "Builds",
    lead: "What a deployment produced: the artifacts, and what identifies them. Immutable, and only ever the result of an attempt that got there.",
    tags: ["v1", "stable"],
    source: "content/collections/neurun/builds.mdx",
    toc: [
      { id: "output", label: "A build is an output" },
      { id: "artifacts", label: "What a build contains" },
      { id: "pinning", label: "Executions pin to one" },
    ],
    body: (
      <>
        <H2 id="output">A build is an output</H2>
        <P>
          A build carries no status and no failure. How the attempt went belongs to the deployment
          that ran it — its stages, its toolchain output, and the reason it stopped. A deployment
          that failed before producing artifacts has no build at all, so the presence of one is
          itself the statement that it worked.
        </P>

        <H2 id="artifacts">What a build contains</H2>
        <P>
          A code layer always, and an install layer when the source has a non-empty{" "}
          <C>requirements.txt</C>. Both are artifacts, addressed by digest. Alongside them the build
          records what runs them: the runtime, the entrypoint, and the digest of the source it was
          made from.
        </P>
        <Snippet language="json" filename="deployment.build">{`{
  "id": "bld_9F3AC41",
  "runtime": "python",
  "entrypoint": "main.py:handler",
  "source_sha256": "9f3a…c41",
  "artifacts": [
    {"kind": "code_layer"},
    {"kind": "install_layer"}
  ]
}`}</Snippet>

        <H2 id="pinning">Executions pin to one</H2>
        <P>
          An execution names the build it ran against and keeps it. Deploying again produces a new
          deployment with a new build; nothing an older execution was pinned to ever changes
          underneath it.
        </P>
      </>
    ),
  },

  executions: {
    slug: "executions",
    label: "Executions",
    group: "Execute",
    title: "Executions",
    lead: "One invocation of a built handler — the durable record of what was sent in, which build answered, and how it ended.",
    tags: ["v1", "stable", "billed"],
    source: "content/collections/neurun/executions.mdx",
    toc: [
      { id: "create", label: "Creating one" },
      { id: "states", label: "States" },
      { id: "pinned", label: "Pinned to a build" },
      { id: "rerun", label: "Rerun" },
      { id: "compute", label: "What is metered" },
    ],
    body: (
      <>
        <H2 id="create">Creating one</H2>
        <P>
          <C>POST /v1/deployments/{"{deployment_id}"}/executions</C> takes an <C>input</C> of any
          shape and returns <C>202</C> with the execution already pinned to the latest ready build. A
          worker picks it up on its own clock; the record holds the request in between.
        </P>
        <Snippet filename="shell">{`curl -sS -i -X POST \\
  $NEURUN_URL/v1/deployments/dep_01HXQ8F2K9/executions \\
  -H "authorization: Bearer $NEURUN_KEY" \\
  -d '{"input":{"url":"https://example.com"}}'

HTTP/1.1 202 Accepted`}</Snippet>

        <H2 id="states">States</H2>
        <P>
          <C>queued</C> → <C>running</C> → <C>succeeded</C> or <C>failed</C>. Use the lowercase API
          values verbatim. An unrecognised value is rendered neutrally with its raw text — never
          guessed into success, never allowed to crash a route.
        </P>
        <Rows
          items={[
            { term: "queued", body: "Accepted and waiting for a worker. Holding no compute, and billed for none." },
            { term: "running", body: "Claimed by a worker. The clock that bills you starts here." },
            { term: "succeeded", body: "Terminal. Output and logs are settled and addressable." },
            { term: "failed", body: "Terminal. Carries a failure code and message alongside whatever logs were written." },
          ]}
        />
        <Callout kind="note" title="Claiming is safe under concurrency">
          A claim takes the oldest queued row with <C>FOR UPDATE SKIP LOCKED</C>, so several workers
          drain the queue without colliding. Finalizing is a compare-and-set: only a running execution
          may go terminal, so a late write from a worker that lost its lease loses.
        </Callout>
        <P>
          Executions a crashed worker left <C>running</C> are marked <C>failed</C> with{" "}
          <C>worker_restarted</C> on the next start. They are never re-run for you — rerunning is your
          decision, because only you know whether the handler is safe to repeat.
        </P>

        <H2 id="pinned">Pinned to a build</H2>
        <P>
          An execution is pinned to the build that was ready when it was created, and never moves. Ten
          rebuilds later it still names the code that ran. That pinning is the whole reason provenance
          is legible here rather than reconstructed from deploy timestamps.
        </P>

        <H2 id="rerun">Rerun</H2>
        <P>
          <C>POST /v1/executions/{"{execution_id}"}/rerun</C> repeats the same input against the same
          build, and refuses if that build is no longer ready rather than quietly running a newer one.
          The new execution records <C>rerun_of_execution_id</C>, so the pair stays legible.
        </P>

        <H2 id="compute">What is metered</H2>
        <P>
          Memory reserved multiplied by wall time, from the moment a worker claims the execution to
          the moment it goes terminal. Queued time is not billed. Builds are not billed. A rerun costs
          whatever it consumes, like any other execution.
        </P>
        <Callout kind="roadmap" title="Servers are metered differently">
          A server holds an app resident and is billed for the time it is up, not per execution. It is
          unbuilt — see <Link href="/docs/servers">Servers</Link>.
        </Callout>
      </>
    ),
  },

  servers: {
    slug: "servers",
    label: "Servers",
    group: "Execute",
    title: "Servers",
    lead: "A machine that holds one app resident and exposes an endpoint, so callers reach the app directly instead of creating an execution per call.",
    tags: ["roadmap", "unbuilt"],
    source: "content/collections/neurun/servers.mdx",
    toc: [
      { id: "shape", label: "The shape" },
      { id: "why", label: "When a server is the right answer" },
      { id: "billing", label: "A different meter" },
      { id: "status", label: "Status" },
    ],
    body: (
      <>
        <Callout kind="roadmap" title="Nothing on this page is available yet">
          Servers are unbuilt. There is no <C>/v1/servers</C> endpoint, no server in the dashboard,
          and no plan that includes one. This page describes the shape being designed so you can tell
          whether to wait for it.
        </Callout>

        <H2 id="shape">The shape</H2>
        <P>
          An app is executed, not hosted: a deployment produces a build, an execution invokes it once,
          and the process goes away. A server inverts that. It pins one app to one ready build, keeps
          it resident on a machine, and exposes an endpoint you call directly — no execution record per
          call, no cold start per call.
        </P>
        <Rows
          items={[
            { term: "execution", body: "One invocation, one record, one pinned build. Billed for the compute it consumed." },
            { term: "server", body: "One resident app behind an endpoint. Billed for the time it is up." },
          ]}
        />

        <H2 id="why">When a server is the right answer</H2>
        <P>
          When startup cost dwarfs the work: a crawler that holds a warm connection pool, a handler
          that loads a large model or ruleset once, anything fronting a latency budget an execution
          queue cannot meet. If your handler starts in milliseconds and runs on a schedule, executions
          are cheaper and leave a better record — stay with them.
        </P>

        <H2 id="billing">A different meter</H2>
        <P>
          Resident time and per-execution compute cannot share a meter without double-counting, so a
          server will be metered and invoiced as its own line. Nothing about the execution model
          changes when servers ship, and no execution price moves because of them.
        </P>

        <H2 id="status">Status</H2>
        <P>
          The dashboard&apos;s <a href="/servers">Servers</a> route names the contracts still
          required: a lifecycle the control plane owns, the exposed endpoint and its authentication,
          resident-time metering, and health and logs for a process that has no execution record to
          attach either to. When those ship, this page stops carrying a roadmap banner.
        </P>
      </>
    ),
  },

  authentication: {
    slug: "authentication",
    label: "Authentication",
    group: "Contracts",
    title: "Authentication",
    lead: "Two credentials reach /v1: a session cookie for a person at a browser, and a bearer API key for a program. Both resolve to scopes.",
    tags: ["v1", "stable"],
    source: "content/collections/neurun/authentication.mdx",
    toc: [
      { id: "register", label: "Registration" },
      { id: "keys", label: "Key format" },
      { id: "scopes", label: "Scopes are the model" },
      { id: "sessions", label: "Dashboard sessions" },
      { id: "storage", label: "Where a key may live" },
      { id: "revoke", label: "Revocation" },
    ],
    body: (
      <>
        <H2 id="register">Registration</H2>
        <P>
          <C>POST /v1/auth/register</C> is public and unauthenticated — it is how a caller obtains
          the credential everything under <C>/v1</C> requires. It creates the account as an{" "}
          <C>admin</C>, creates the project it names, and sets the session cookie. Later accounts are
          invited instead, through <C>POST /v1/users</C>, which takes a role and needs{" "}
          <C>users:write</C>.
        </P>
        <Callout kind="note" title="Rate limiting belongs at the edge">
          Sign-up is open, so put per-IP limiting in front of the server. It deliberately holds no
          limiter of its own, matching the sign-in throttle that was removed for the same reason.
        </Callout>

        <H2 id="keys">Key format</H2>
        <Snippet filename="http">{`Authorization: Bearer neu_<environment>_<prefix>.<secret>`}</Snippet>
        <P>
          The part before the dot is the indexed lookup prefix; the comparison against the stored
          digest is constant time. Only a SHA-256 digest is stored, so a key you did not record at
          creation is gone — issue a new one rather than hunting for it.
        </P>

        <H2 id="scopes">Scopes are the model</H2>
        <P>
          A key is not attached to a project. Scopes are the entire authorization model, so pinning a
          key to one project would both duplicate that decision and cap the key for no reason.
          Projects scope resources, not callers.
        </P>
        <Snippet language="json">{`["apps:read", "apps:write",
 "deployments:read", "deployments:write",
 "builds:read",
 "executions:read", "executions:write",
 "projects:read", "projects:write",
 "users:read", "users:write",
 "api_keys:read", "api_keys:write",
 "*"]`}</Snippet>
        <Callout kind="note" title="A limited key cannot mint an unlimited one">
          A key can never be granted a scope its creator does not already hold. <C>*</C> covers
          everything, including scopes added in later releases.
        </Callout>

        <H2 id="sessions">Dashboard sessions</H2>
        <P>
          <C>POST /v1/auth/login</C> answers with an <C>HttpOnly</C>, <C>Secure</C>,{" "}
          <C>SameSite=Strict</C> cookie, so the dashboard holds no credential a script could read.{" "}
          <C>GET /v1/auth/session</C> returns the dashboard, their role and their scopes;{" "}
          <C>POST /v1/auth/logout</C> ends it. A signed-in user is unrestricted by project — roles are{" "}
          <C>admin</C>, <C>operator</C> and <C>viewer</C>.
        </P>

        <H2 id="storage">Where a key may live</H2>
        <Rows
          items={[
            { term: "allowed", body: "A server-side secret store, a CI secret, an environment variable on the machine that calls the API." },
            { term: "refused", body: "localStorage, a query string, a URL fragment, a breadcrumb, a log line, or anything the browser persists across tabs." },
          ]}
        />

        <H2 id="revoke">Revocation</H2>
        <P>
          <C>DELETE /v1/api-keys/{"{id}"}</C> sets <C>revoked_at</C> and is idempotent — revoking
          twice keeps the first timestamp. Nothing restores a revoked key; no configuration re-asserts
          it at boot. Deleting the user who created a key leaves the key working, with its attribution
          cleared.
        </P>
      </>
    ),
  },

  errors: {
    slug: "errors",
    label: "Error model",
    group: "Contracts",
    title: "Error model",
    lead: "One envelope for every failure, a stable code to branch on, and a request id to quote when you ask for help.",
    tags: ["v1", "stable"],
    source: "content/collections/neurun/errors.mdx",
    toc: [
      { id: "envelope", label: "The envelope" },
      { id: "branch", label: "Branch on the code" },
      { id: "pagination", label: "Pagination" },
      { id: "unknown", label: "Unknown values" },
    ],
    body: (
      <>
        <H2 id="envelope">The envelope</H2>
        <P>
          Every non-2xx response carries the same shape. <C>message</C> is prose written for a person
          and may change between releases; <C>code</C> is the stable part.
        </P>
        <Split>
          <Panel flush label="Refused" actions={<Meta>422</Meta>}>
            <Snippet language="json">{`{
  "error": {
    "code": "validation_failed",
    "message": "runtime must be python",
    "details": {"field": "runtime"}
  }
}`}</Snippet>
          </Panel>
          <Panel flush label="Missing" actions={<Meta>404</Meta>}>
            <Snippet language="json">{`{
  "error": {
    "code": "app_not_found",
    "message": "app not found"
  }
}`}</Snippet>
          </Panel>
        </Split>

        <H2 id="branch">Branch on the code</H2>
        <P>
          Never parse <C>message</C>, and never branch on the status code alone — several distinct
          refusals share a status.
        </P>
        <Rows
          items={[
            { term: "401", body: "No credential, or one the server does not recognise." },
            { term: "403", body: "Authenticated, but the scope required is not held." },
            { term: "404", body: "The resource does not exist, or is outside the scopes held." },
            { term: "413", body: "The repository source exceeded the accepted size." },
            { term: "422", body: "The request was understood and refused. Read details." },
          ]}
        />

        <H2 id="pagination">Pagination</H2>
        <P>
          List endpoints take <C>limit</C> — between 1 and 200, defaulting to 50 — and return a
          single named array. Server-side filters are deliberately narrow: <C>app_id</C> on
          deployments, <C>deployment_id</C> on executions. Anything else belongs in the client, on the
          page you already fetched.
        </P>

        <H2 id="unknown">Unknown values</H2>
        <Callout kind="warning" title="Render what you were sent">
          A status, runtime or failure code you do not recognise is a server that is newer than your
          client. Show the raw value neutrally. Do not coerce it into the nearest state you do know —
          an unknown status displayed as <C>succeeded</C> is the worst bug this system can have.
        </Callout>
      </>
    ),
  },
};

function Meta({ children }: { children: ReactNode }) {
  return <span className="font-mono text-micro text-fg-muted">{children}</span>;
}

export function docsTags(tags: string[]) {
  return tags.map((tag) => (
    <Badge key={tag} variant={tag === "roadmap" || tag === "unbuilt" ? "dotted" : "outline"}>
      {tag}
    </Badge>
  ));
}

export function docsNeighbours(slug: string) {
  const index = DOCS_ORDER.indexOf(slug);
  return {
    prev: index > 0 ? DOCS[DOCS_ORDER[index - 1]] : undefined,
    next: index >= 0 && index < DOCS_ORDER.length - 1 ? DOCS[DOCS_ORDER[index + 1]] : undefined,
  };
}
