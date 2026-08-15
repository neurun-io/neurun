"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import { Play } from "lucide-react";
import { toast } from "sonner";

import { InlineError } from "@/components/neurun/error-panel";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useBuildsQuery, useCreateExecutionMutation } from "@/lib/api/queries";

/** Runs the app: its latest build, or one picked from what it has produced. */
export function RunPanel({ appId }: { appId: string }) {
  const builds = useBuildsQuery();
  const create = useCreateExecutionMutation();
  const router = useRouter();
  const [input, setInput] = useState("{}");
  const [buildId, setBuildId] = useState("latest");
  const [parseError, setParseError] = useState<string | null>(null);

  const available = (builds.data?.builds ?? []).filter((build) => build.app_id === appId);

  function submit(event: FormEvent) {
    event.preventDefault();
    let parsed: unknown;
    try {
      parsed = JSON.parse(input);
      setParseError(null);
    } catch {
      setParseError("Input must be valid JSON.");
      return;
    }
    create.mutate(
      { appId, input: parsed, buildId: buildId === "latest" ? undefined : buildId },
      {
        onSuccess: (execution) => {
          toast.success(`Queued ${execution.id}`);
          router.push(`/executions/${execution.id}`);
        },
      },
    );
  }

  return (
    <Panel label="Run">
      <form onSubmit={submit} className="space-y-3">
        <div className="space-y-1.5">
          <Label htmlFor="run-build">Build</Label>
          <Select value={buildId} onValueChange={setBuildId}>
            <SelectTrigger id="run-build" className="font-mono">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="latest">Latest ready</SelectItem>
              {available.map((build) => (
                <SelectItem key={build.id} value={build.id} className="font-mono">
                  {build.id}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="run-input">JSON input</Label>
          <Textarea
            id="run-input"
            className="min-h-36 font-mono"
            value={input}
            onChange={(event) => setInput(event.target.value)}
          />
        </div>
        {parseError ? <p className="text-sm text-destructive">{parseError}</p> : null}
        {create.isError ? <InlineError error={create.error} /> : null}
        <Button disabled={create.isPending}>
          <Play aria-hidden className="size-3.5" strokeWidth={1.5} />
          {create.isPending ? "Queuing…" : "Run"}
        </Button>
      </form>
    </Panel>
  );
}
