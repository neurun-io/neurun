"use client";

import { Suspense, useState } from "react";
import { useRouter, useSearchParams } from "next/navigation";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";

import { InstallationPanel } from "@/components/github/installation-panel";
import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { KeyValue } from "@/components/neurun/key-value";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import * as api from "@/lib/api/endpoints";
import { SESSION_QUERY_KEY, useSession } from "@/lib/session/store";

const ORGANIZATIONS_QUERY_KEY = ["neurun", "organizations"] as const;

/** One row per organization: `name.id`, valued `<name>.<id>`. */
function organizationRows(records: api.OrganizationSummary[]) {
  return records.map((record) => ({
    label: "name.id",
    value: (
      <span className="font-mono text-caption">
        {record.name}.{record.id}
      </span>
    ),
  }));
}

/**
 * An invitation arrives one of two ways: pasted as a token, or followed as a
 * link carrying `?invite=`. Both spend it through the same `acceptInvite` call —
 * the link only saves the recipient a copy and paste.
 */
function OrganizationView() {
  const router = useRouter();
  const queryClient = useQueryClient();
  const { session } = useSession();
  const linkedToken = useSearchParams().get("invite")?.trim() ?? "";
  const [typedToken, setTypedToken] = useState("");

  const organizations = useQuery({
    queryKey: ORGANIZATIONS_QUERY_KEY,
    queryFn: ({ signal }) => api.listOrganizations(signal),
  });

  // Name what the link would join before spending the token on it.
  const preview = useQuery({
    queryKey: ["neurun", "invite-preview", linkedToken],
    queryFn: ({ signal }) => api.lookupInvite(linkedToken, signal),
    enabled: linkedToken !== "",
    retry: false,
  });

  const join = useMutation({
    mutationFn: (token: string) => api.acceptInvite(token),
    onSuccess: async () => {
      toast.success("Joined organization");
      setTypedToken("");
      await queryClient.invalidateQueries({ queryKey: SESSION_QUERY_KEY });
      await queryClient.invalidateQueries({ queryKey: ORGANIZATIONS_QUERY_KEY });
      if (linkedToken) router.replace("/organization");
    },
  });

  const all = organizations.data ?? [];
  const owned = all.filter((record) => record.owner_user_id === session?.user_id);
  const joined = all.filter((record) => record.owner_user_id !== session?.user_id);

  return (
    <div>
      <PageHeader
        title="Organization"
        description="What this account owns, what it has joined, and how to join another."
      />
      <div className="space-y-4 p-6">
        {organizations.isError ? (
          <ErrorPanel error={organizations.error} onRetry={() => organizations.refetch()} />
        ) : organizations.isPending ? (
          <Panel label="Owned">
            <p className="text-fg-muted">Loading…</p>
          </Panel>
        ) : (
          <>
            <Panel label="Owned">
              {owned.length === 0 ? (
                <p className="text-sm text-fg-muted">
                  This account owns no organization. An account may own one.
                </p>
              ) : (
                <div className="space-y-4">
                  <KeyValue rows={organizationRows(owned)} />
                  <InstallationPanel />
                </div>
              )}
            </Panel>

            {joined.length > 0 ? (
              <Panel label="Joined">
                <KeyValue rows={organizationRows(joined)} />
              </Panel>
            ) : null}
          </>
        )}

        {linkedToken ? (
          <Panel label="Invitation">
            {preview.isPending ? (
              <p className="text-fg-muted">Reading the invitation…</p>
            ) : preview.isError ? (
              <InlineError error={preview.error} />
            ) : (
              <div className="space-y-3">
                <p className="text-sm text-fg-secondary">
                  You were invited to{" "}
                  <span className="text-fg">{preview.data.organization.name}</span> as{" "}
                  <span className="text-fg">{preview.data.role}</span>, at{" "}
                  <span className="font-mono text-caption">{preview.data.email}</span>.
                </p>
                {join.error ? <InlineError error={join.error} /> : null}
                <Button disabled={join.isPending} onClick={() => join.mutate(linkedToken)}>
                  {join.isPending ? (
                    <>
                      <Loader2 aria-hidden className="size-3.5 animate-spin" strokeWidth={1.5} />
                      Joining
                    </>
                  ) : (
                    "Join organization"
                  )}
                </Button>
              </div>
            )}
          </Panel>
        ) : (
          <Panel label="Join another">
            <form
              onSubmit={(event) => {
                event.preventDefault();
                join.mutate(typedToken.trim());
              }}
              className="flex max-w-115 flex-col gap-3"
            >
              <div className="flex flex-col gap-1.5">
                <Label htmlFor="invite-token" className="nr-label">
                  Invitation token
                </Label>
                <Input
                  id="invite-token"
                  value={typedToken}
                  onChange={(event) => setTypedToken(event.target.value)}
                  autoCapitalize="none"
                  spellCheck={false}
                  required
                  className="font-mono text-caption"
                />
                <p className="text-micro text-fg-muted">
                  Or follow the invitation link, which carries the token and joins in one step. An
                  invitation must have been issued to your address, and expires seven days after it
                  was sent.
                </p>
              </div>
              {join.error ? <InlineError error={join.error} /> : null}
              <Button type="submit" disabled={join.isPending} className="self-start">
                {join.isPending ? (
                  <>
                    <Loader2 aria-hidden className="size-3.5 animate-spin" strokeWidth={1.5} />
                    Joining
                  </>
                ) : (
                  "Join organization"
                )}
              </Button>
            </form>
          </Panel>
        )}
      </div>
    </div>
  );
}

export default function OrganizationPage() {
  return (
    <Suspense fallback={null}>
      <OrganizationView />
    </Suspense>
  );
}
