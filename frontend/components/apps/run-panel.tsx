"use client";

import { useState, type FormEvent } from "react";
import { useRouter } from "next/navigation";
import Link from "next/link";
import { Play } from "lucide-react";
import { toast } from "sonner";

import { InlineError } from "@/components/neurun/error-panel";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import { useAppQuery, useCreateExecutionMutation } from "@/lib/api/queries";

/** Runs the app on the build it is active on. */
export function RunPanel({ appId }: { appId: string }) {
  const app = useAppQuery(appId);
  const create = useCreateExecutionMutation();
  const router = useRouter();
  const [input, setInput] = useState("{}");
  const [parseError, setParseError] = useState<string | null>(null);

  const active = app.data?.active_build_id;

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
      { appId, input: parsed },
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
        <div className="flex items-center gap-3">
          <Button disabled={create.isPending}>
            <Play aria-hidden className="size-3.5" strokeWidth={1.5} />
            {create.isPending ? "Queuing…" : "Run"}
          </Button>
          <span className="nr-label">
            {active ? (
              <Link className="font-mono underline" href={`/builds/${active}`}>
                {active}
              </Link>
            ) : (
              "Newest build"
            )}
          </span>
        </div>
      </form>
    </Panel>
  );
}
