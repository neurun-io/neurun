"use client";

import Link from "next/link";

import { ErrorPanel } from "@/components/neurun/error-panel";
import { EmptyState } from "@/components/neurun/feedback";
import { PageHeader } from "@/components/neurun/page-header";
import { Panel } from "@/components/neurun/panel";
import { Timestamp } from "@/components/neurun/timestamp";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useProjectsQuery } from "@/lib/api/queries";

export default function ProjectsPage() {
  const query = useProjectsQuery();

  return (
    <div>
      <PageHeader
        title="Projects"
        description="The ownership boundary for Apps, deployments, builds, executions, users, and API keys."
      />
      <div className="p-6">
        {query.isError ? (
          <ErrorPanel error={query.error} onRetry={() => query.refetch()} />
        ) : (
          <Panel label="Projects" flush>
            {query.isPending ? (
              <p className="p-4 text-fg-muted">Loading…</p>
            ) : query.data.projects.length === 0 ? (
              <EmptyState title="No projects" description="No project is available to this user." />
            ) : (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Project</TableHead>
                    <TableHead>Name</TableHead>
                    <TableHead>Updated</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {query.data.projects.map((project) => (
                    <TableRow key={project.id}>
                      <TableCell>
                        <Link className="font-mono underline" href={`/projects/${project.id}`}>
                          {project.id}
                        </Link>
                      </TableCell>
                      <TableCell>{project.name}</TableCell>
                      <TableCell>
                        <Timestamp value={project.updated_at} />
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
