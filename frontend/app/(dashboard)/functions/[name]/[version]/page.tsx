"use client";

import { useParams } from "next/navigation";

import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Digest } from "@/components/neurun/copy-id";
import { KeyValue } from "@/components/neurun/key-value";
import { JsonView } from "@/components/neurun/json-view";
import { Callout } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { InvocationForm } from "@/components/functions/invocation-form";
import { useFunctionVersionQuery } from "@/lib/api/queries";
import { acceptsAsyncExecution } from "@/lib/api/types";

/**
 * Manifest detail plus a direct-invocation form.
 *
 * Several manifest objects — retry, redaction, telemetry, artifacts,
 * resource_policy — are still open extension maps in the contract. They are
 * shown verbatim as unknown records rather than being given a typed UI that
 * would imply a shape the backend has not committed to.
 */
export default function FunctionVersionPage() {
  const params = useParams<{ name: string; version: string }>();
  const name = decodeURIComponent(params.name);
  const version = decodeURIComponent(params.version);

  const query = useFunctionVersionQuery(name, version);

  if (query.isError) {
    return (
      <div className="px-6 py-6">
        <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
      </div>
    );
  }

  if (query.isPending) {
    return (
      <div className="space-y-4 px-6 py-6" role="status" aria-label="Loading function">
        <Skeleton className="h-8 w-72" />
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }

  const manifest = query.data;
  const openMaps: [string, unknown][] = (
    [
      ["retry", manifest.retry],
      ["redaction", manifest.redaction],
      ["telemetry", manifest.telemetry],
      ["artifacts", manifest.artifacts],
      ["resource_policy", manifest.resource_policy],
    ] satisfies [string, unknown][]
  ).filter(([, value]) => value !== undefined && Object.keys(value ?? {}).length > 0);

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        crumbs={[
          { label: "Functions", href: "/functions" },
          { label: manifest.name, href: `/functions/${manifest.name}/${manifest.version}` },
          { label: manifest.version },
        ]}
        title={
          <span className="font-mono">
            {manifest.name}
            <span className="text-fg-muted">@{manifest.version}</span>
          </span>
        }
        description={manifest.description}
        meta={
          <>
            <Digest value={manifest.digest} />
            <Badge variant="secondary">{manifest.category}</Badge>
            <Badge variant="secondary">{manifest.execution_context}</Badge>
            <Badge variant="secondary">{manifest.side_effects}</Badge>
            <Badge variant={acceptsAsyncExecution(manifest) ? "outline" : "dotted"}>
              {acceptsAsyncExecution(manifest) ? "sync + async" : "sync only"}
            </Badge>
          </>
        }
      />

      <div className="grid min-w-0 flex-1 gap-4 px-6 py-4 xl:grid-cols-[minmax(0,1fr)_420px]">
        <div className="min-w-0 space-y-4">
          <Panel label="Input schema">
            <JsonView value={manifest.input_schema} preRedacted />
          </Panel>

          <Panel label="Output schema">
            <JsonView value={manifest.output_schema} preRedacted />
          </Panel>

          {openMaps.length > 0 ? (
            <Panel
              label="Manifest policy"
              footer="Open extension maps in the current contract — shown verbatim until fully typed component schemas are published."
            >
              <div className="space-y-3">
                {openMaps.map(([key, value]) => (
                  <section key={key}>
                    <p className="nr-label mb-1.5">{key}</p>
                    <JsonView value={value} preRedacted />
                  </section>
                ))}
              </div>
            </Panel>
          ) : null}
        </div>

        <aside className="min-w-0 space-y-4">
          <Panel label="Contract">
            <KeyValue
              rows={[
                { label: "Version", value: manifest.version },
                { label: "Digest", value: <Digest value={manifest.digest} /> },
                { label: "Category", value: manifest.category },
                { label: "Execution context", value: manifest.execution_context },
                { label: "Side effects", value: manifest.side_effects },
                { label: "Default timeout", value: `${manifest.timeout.default_ms} ms` },
                { label: "Maximum timeout", value: `${manifest.timeout.maximum_ms} ms` },
              ]}
            />
          </Panel>

          {manifest.capabilities?.length || manifest.permissions?.length ? (
            <Panel label="Capabilities">
              <div className="flex flex-wrap gap-1.5">
                {(manifest.capabilities ?? []).map((capability) => (
                  <Badge key={capability} variant="tag">
                    {capability}
                  </Badge>
                ))}
                {(manifest.permissions ?? []).map((permission) => (
                  <Badge key={permission} variant="outline">
                    {permission}
                  </Badge>
                ))}
              </div>
            </Panel>
          ) : null}

          <InvocationForm manifest={manifest} />

          <Callout kind="roadmap" title="Compatibility versions">
            Which server and agent builds accept this manifest is a future contract field.
          </Callout>
        </aside>
      </div>
    </div>
  );
}
