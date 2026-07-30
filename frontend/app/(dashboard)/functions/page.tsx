"use client";

import { useMemo, useState } from "react";
import Link from "next/link";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Digest } from "@/components/neurun/copy-id";
import { EmptyState } from "@/components/neurun/feedback";
import { ErrorPanel } from "@/components/neurun/error-panel";
import { useFunctionsQuery } from "@/lib/api/queries";
import { acceptsAsyncExecution } from "@/lib/api/types";

const ANY = "__any__";

/**
 * The built-in, release-owned catalog.
 *
 * `category` and `capability` are server-side filters the contract publishes.
 * Execution context and side-effect class are applied locally to the returned
 * manifests — the manifests are immutable and the whole catalog arrives in one
 * uncursored response, so filtering here cannot misrepresent completeness.
 *
 * `status` is not exposed as a control: the current release accepts only
 * `available`, so a lifecycle filter would be a control with one setting.
 */
export default function FunctionsPage() {
  const [category, setCategory] = useState<string>(ANY);
  const [capability, setCapability] = useState<string>(ANY);
  const [executionContext, setExecutionContext] = useState<string>(ANY);
  const [sideEffects, setSideEffects] = useState<string>(ANY);
  const [search, setSearch] = useState("");

  const query = useFunctionsQuery({
    category: category === ANY ? undefined : category,
    capability: capability === ANY ? undefined : capability,
  });

  const manifests = useMemo(() => query.data?.functions ?? [], [query.data]);

  const { categories, capabilities, contexts, effects } = useMemo(() => {
    return {
      categories: unique(manifests.map((m) => m.category)),
      capabilities: unique(manifests.flatMap((m) => m.capabilities ?? [])),
      contexts: unique(manifests.map((m) => m.execution_context)),
      effects: unique(manifests.map((m) => m.side_effects)),
    };
  }, [manifests]);

  const visible = useMemo(() => {
    const needle = search.trim().toLowerCase();
    return manifests.filter((manifest) => {
      if (executionContext !== ANY && manifest.execution_context !== executionContext) return false;
      if (sideEffects !== ANY && manifest.side_effects !== sideEffects) return false;
      if (needle && !`${manifest.name} ${manifest.description ?? ""}`.toLowerCase().includes(needle))
        return false;
      return true;
    });
  }, [manifests, executionContext, sideEffects, search]);

  return (
    <div className="flex min-h-full flex-col">
      <PageHeader
        title="Functions"
        description="The installed catalog is release-owned, immutable and read-only. Every version is digest-pinned."
      />

      <div className="space-y-4 px-6 py-4">
        <Panel label="Filters">
          <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
            <div className="space-y-1.5 lg:col-span-1">
              <Label htmlFor="function-search">Search</Label>
              <Input
                id="function-search"
                value={search}
                onChange={(event) => setSearch(event.target.value)}
                placeholder="name or description"
                className="font-mono text-caption"
              />
            </div>
            <FilterSelect
              id="filter-category"
              label="Category"
              value={category}
              onChange={setCategory}
              options={categories}
            />
            <FilterSelect
              id="filter-capability"
              label="Capability"
              value={capability}
              onChange={setCapability}
              options={capabilities}
            />
            <FilterSelect
              id="filter-context"
              label="Execution context"
              value={executionContext}
              onChange={setExecutionContext}
              options={contexts}
            />
            <FilterSelect
              id="filter-effects"
              label="Side effects"
              value={sideEffects}
              onChange={setSideEffects}
              options={effects}
            />
          </div>
        </Panel>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : query.isPending ? (
          <div className="grid gap-3 md:grid-cols-2">
            {Array.from({ length: 6 }).map((_, index) => (
              <Skeleton key={index} className="h-28 w-full" />
            ))}
          </div>
        ) : visible.length === 0 ? (
          <Panel label="Catalog">
            <EmptyState
              title="No functions match this filter"
              description="Clear a filter to widen the catalog."
              action={
                <Button
                  variant="secondary"
                  size="sm"
                  onClick={() => {
                    setCategory(ANY);
                    setCapability(ANY);
                    setExecutionContext(ANY);
                    setSideEffects(ANY);
                    setSearch("");
                  }}
                >
                  Clear filters
                </Button>
              }
            />
          </Panel>
        ) : (
          <ul className="grid gap-3 md:grid-cols-2">
            {visible.map((manifest) => (
              <li key={`${manifest.name}@${manifest.version}`}>
                <Link
                  href={`/functions/${manifest.name}/${manifest.version}`}
                  className="block h-full rounded-lg transition-colors duration-120 ease-mech hover:bg-surface-raised"
                >
                  <Panel
                    label={manifest.category}
                    actions={
                      <Badge variant={acceptsAsyncExecution(manifest) ? "outline" : "dotted"}>
                        {acceptsAsyncExecution(manifest) ? "sync + async" : "sync only"}
                      </Badge>
                    }
                    className="h-full bg-transparent"
                  >
                    <div className="space-y-2">
                      <p className="font-mono text-sm text-fg">
                        {manifest.name}
                        <span className="text-fg-muted">@{manifest.version}</span>
                      </p>
                      {manifest.description ? (
                        <p className="line-clamp-2 text-caption text-fg-secondary">
                          {manifest.description}
                        </p>
                      ) : null}
                      <div className="flex flex-wrap items-center gap-1.5">
                        <Badge variant="secondary">{manifest.execution_context}</Badge>
                        <Badge variant="secondary">{manifest.side_effects}</Badge>
                      </div>
                      <Digest value={manifest.digest} />
                    </div>
                  </Panel>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

function FilterSelect({
  id,
  label,
  value,
  onChange,
  options,
}: {
  id: string;
  label: string;
  value: string;
  onChange: (value: string) => void;
  options: string[];
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Select value={value} onValueChange={onChange}>
        <SelectTrigger id={id} className="w-full font-mono text-caption">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          <SelectItem value={ANY}>any</SelectItem>
          {options.map((option) => (
            <SelectItem key={option} value={option} className="font-mono text-caption">
              {option}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}

function unique(values: string[]): string[] {
  return Array.from(new Set(values.filter(Boolean))).sort();
}
