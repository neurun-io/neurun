"use client";

import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorPanel, InlineError } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useCreateUserMutation, useUpdateUserMutation, useUsersQuery } from "@/lib/api/queries";
import type { UserRole } from "@/lib/api/resource-types";

export default function UsersPage() {
  const query = useUsersQuery();
  const create = useCreateUserMutation();
  const update = useUpdateUserMutation();
  const [username, setUsername] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [role, setRole] = useState<UserRole>("operator");
  const [password, setPassword] = useState("");

  function submit(event: FormEvent) {
    event.preventDefault();
    create.mutate(
      { username, display_name: displayName, role, password },
      {
        onSuccess: () => {
          setUsername("");
          setDisplayName("");
          setPassword("");
          toast.success("User created");
        },
      },
    );
  }

  return (
    <div>
      <PageHeader title="Users" description="People who can sign in and own scoped API keys." />
      <div className="space-y-4 p-6">
        <Panel label="New user">
          <form onSubmit={submit} className="grid gap-3 md:grid-cols-2">
            <Field id="username" label="Username" value={username} setValue={setUsername} />
            <Field
              id="display-name"
              label="Display name"
              value={displayName}
              setValue={setDisplayName}
            />
            <div className="space-y-1.5">
              <Label htmlFor="role">Role</Label>
              <Select value={role} onValueChange={(value) => setRole(value as UserRole)}>
                <SelectTrigger id="role">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value="admin">admin</SelectItem>
                  <SelectItem value="operator">operator</SelectItem>
                  <SelectItem value="viewer">viewer</SelectItem>
                </SelectContent>
              </Select>
            </div>
            <Field
              id="password"
              label="Password"
              value={password}
              setValue={setPassword}
              type="password"
            />
            <div className="md:col-span-2">
              {create.isError ? <InlineError error={create.error} /> : null}
              <Button className="mt-2" disabled={create.isPending}>
                Create user
              </Button>
            </div>
          </form>
        </Panel>

        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Users" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.users.length === 0 ? (
              <EmptyState title="No users" description="Create the first user above." />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>User</TableHead>
                    <TableHead>Display name</TableHead>
                    <TableHead>Role</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.users.map((user) => (
                    <TableRow key={user.id}>
                      <TableCell>
                        <div className="font-medium">{user.username}</div>
                        <div className="font-mono text-micro text-fg-muted">{user.id}</div>
                      </TableCell>
                      <TableCell>{user.display_name}</TableCell>
                      <TableCell>{user.role}</TableCell>
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

function Field({
  id,
  label,
  value,
  setValue,
  type = "text",
}: {
  id: string;
  label: string;
  value: string;
  setValue: (value: string) => void;
  type?: string;
}) {
  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        type={type}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        required
      />
    </div>
  );
}
