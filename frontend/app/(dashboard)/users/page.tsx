"use client";

import Link from "next/link";
import { toast } from "sonner";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { Callout, EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useUpdateUserMutation, useUsersQuery } from "@/lib/api/queries";

export default function UsersPage() {
  const query = useUsersQuery();
  const update = useUpdateUserMutation();

  return (
    <div>
      <PageHeader
        title="Users"
        description="People in this organization. They arrive by invitation."
      />
      <div className="space-y-4 p-6">
        <Callout kind="note" title="Accounts are not created here">
          Somebody joins by accepting an invitation, which is what sets their role. There is no
          endpoint that mints an account with a password on their behalf.{" "}
          <Link href="/settings/identities">Send an invitation</Link>.
        </Callout>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Users" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.users.length === 0 ? (
              <EmptyState
                title="No users"
                description="Invite somebody to give them access."
              />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>User</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell>
                        <div className="font-medium">{user.email}</div>
                        <div className="font-mono text-micro text-fg-muted">{user.id}</div>
                      </TableCell>
                      <TableCell>{user.disabled ? "disabled" : "active"}</TableCell>
                      <TableCell className="text-right">
                        <Button
                          variant="secondary"
                          size="sm"
                          disabled={update.isPending}
                          onClick={() =>
                            update.mutate(
                              { id: user.id, body: { disabled: !user.disabled } },
                              {
                                onSuccess: () =>
                                  toast.success(user.disabled ? "User enabled" : "User disabled"),
                              },
                            )
                          }
                        >
                          {user.disabled ? "Enable" : "Disable"}
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            )}
          </Panel>
        )}
      </div>
    </div>
  );
}
