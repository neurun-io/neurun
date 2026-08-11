"use client";

/**
 * Query and mutation hooks.
 *
 * Every key is prefixed with the session scope (project + dashboard ID) so cached
 * evidence can never bleed between users. Mutations invalidate explicitly —
 * nothing here relies on a blanket refetch to eventually become correct.
 */
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { sessionScope, useSession } from "@/lib/session/store";
import * as api from "./endpoints";
import * as resources from "./resources";
import { LIVE_POLL_INTERVAL_MS } from "./query-client";
import { isTerminalExecutionStatus } from "./resource-types";

/* -------------------------------------------------------------------------- */
/* Query keys                                                                  */
/* -------------------------------------------------------------------------- */

/**
 * Query keys are built from the session scope rather than a credential, so
 * nothing in a cache key can ever contain a secret.
 */
export function useScope(): string {
  const { session } = useSession();
  return sessionScope(session);
}

export const queryKeys = {
  scope: (scope: string) => ["neurun", scope] as const,

  version: (scope: string) => [...queryKeys.scope(scope), "version"] as const,

  projects: (scope: string) => [...queryKeys.scope(scope), "projects"] as const,
  project: (scope: string, id: string) => [...queryKeys.projects(scope), id] as const,
  apps: (scope: string) => [...queryKeys.scope(scope), "apps"] as const,
  app: (scope: string, id: string) => [...queryKeys.apps(scope), id] as const,
  deployments: (scope: string, appId?: string) =>
    [...queryKeys.scope(scope), "deployments", appId ?? "all"] as const,
  deployment: (scope: string, id: string) =>
    [...queryKeys.scope(scope), "deployment", id] as const,
  builds: (scope: string, deploymentId?: string) =>
    [...queryKeys.scope(scope), "builds", deploymentId ?? "all"] as const,
  build: (scope: string, id: string) => [...queryKeys.scope(scope), "build", id] as const,
  executions: (scope: string, deploymentId?: string) =>
    [...queryKeys.scope(scope), "executions", deploymentId ?? "all"] as const,
  execution: (scope: string, id: string) =>
    [...queryKeys.scope(scope), "execution", id] as const,
  users: (scope: string) => [...queryKeys.scope(scope), "users"] as const,
  apiKeys: (scope: string) => [...queryKeys.scope(scope), "api-keys"] as const,

  browserProfiles: (scope: string) =>
    [...queryKeys.scope(scope), "browser-profiles"] as const,
  browserProfileState: (scope: string, id: string) =>
    [...queryKeys.browserProfiles(scope), id, "state"] as const,

} as const;

/* -------------------------------------------------------------------------- */
/* Health                                                                      */
/* -------------------------------------------------------------------------- */

export function useVersionQuery(options?: { enabled?: boolean }) {
  const scope = useScope();
  const { session } = useSession();

  return useQuery({
    queryKey: queryKeys.version(scope),
    queryFn: async ({ signal }) => (await api.getVersion(signal)).data,
    enabled: session !== null && (options?.enabled ?? true),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
}

/* -------------------------------------------------------------------------- */
/* Projects, deployments, builds, and executions                              */
/* -------------------------------------------------------------------------- */

export function useProjectsQuery() {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.projects(scope),
    queryFn: async ({ signal }) => (await resources.listProjects(signal)).data,
  });
}

export function useProjectQuery(id: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.project(scope, id),
    queryFn: async ({ signal }) => (await resources.getProject(id, signal)).data,
    enabled: Boolean(id),
  });
}

export function useAppsQuery() {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.apps(scope),
    queryFn: async ({ signal }) => (await resources.listApps(signal)).data,
  });
}

export function useAppQuery(id: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.app(scope, id),
    queryFn: async ({ signal }) => (await resources.getApp(id, signal)).data,
    enabled: Boolean(id),
  });
}

export function useDeploymentsQuery(appId?: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.deployments(scope, appId),
    queryFn: async ({ signal }) => (await resources.listDeployments(appId, signal)).data,
    refetchInterval: LIVE_POLL_INTERVAL_MS,
  });
}

export function useDeploymentQuery(id: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.deployment(scope, id),
    queryFn: async ({ signal }) => (await resources.getDeployment(id, signal)).data,
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return !status || status === "uploaded" || status === "building"
        ? LIVE_POLL_INTERVAL_MS
        : false;
    },
  });
}

export function useBuildsQuery(deploymentId?: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.builds(scope, deploymentId),
    queryFn: async ({ signal }) => (await resources.listBuilds(deploymentId, signal)).data,
    refetchInterval: LIVE_POLL_INTERVAL_MS,
  });
}

export function useBuildQuery(id: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.build(scope, id),
    queryFn: async ({ signal }) => (await resources.getBuild(id, signal)).data,
    enabled: Boolean(id),
    refetchInterval: (query) =>
      query.state.data?.status === "building" ? LIVE_POLL_INTERVAL_MS : false,
  });
}

export function useExecutionsQuery(deploymentId?: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.executions(scope, deploymentId),
    queryFn: async ({ signal }) => (await resources.listExecutions(deploymentId, signal)).data,
    refetchInterval: LIVE_POLL_INTERVAL_MS,
  });
}

export function useExecutionQuery(id: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.execution(scope, id),
    queryFn: async ({ signal }) => (await resources.getExecution(id, signal)).data,
    enabled: Boolean(id),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      return status && isTerminalExecutionStatus(status) ? false : LIVE_POLL_INTERVAL_MS;
    },
  });
}

export function useUsersQuery() {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.users(scope),
    queryFn: async ({ signal }) => (await resources.listUsers(signal)).data,
  });
}

export function useAPIKeysQuery() {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.apiKeys(scope),
    queryFn: async ({ signal }) => (await resources.listAPIKeys(signal)).data,
  });
}

export function useUpdateProjectMutation(id: string) {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name }: { name: string }) => (await resources.updateProject(id, name)).data,
    onSuccess: (project) => {
      queryClient.setQueryData(queryKeys.project(scope, id), project);
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects(scope) });
    },
  });
}

export function useCreateProjectMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name }: { name: string }) =>
      (await resources.createProject(name)).data,
    onSuccess: (project) => {
      queryClient.setQueryData(queryKeys.project(scope, project.id), project);
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects(scope) });
    },
  });
}

/**
 * Deleting a project cascades to its apps, deployments, builds and executions,
 * so every one of those caches is dropped rather than invalidated — refetching
 * a deleted subtree would only produce a wave of 404s.
 */
export function useDeleteProjectMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id }: { id: string }) => {
      await resources.deleteProject(id);
      return id;
    },
    onSuccess: (id) => {
      queryClient.removeQueries({ queryKey: queryKeys.project(scope, id) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.projects(scope) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.apps(scope) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.deployments(scope) });
    },
  });
}

export function useCreateAppMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ projectId, name }: { projectId: string; name: string }) =>
      (await resources.createApp(projectId, name)).data,
    onSuccess: (app) => {
      queryClient.setQueryData(queryKeys.app(scope, app.id), app);
      void queryClient.invalidateQueries({ queryKey: queryKeys.apps(scope) });
    },
  });
}

/** Cascades to the app's deployments, builds and executions. */
export function useDeleteAppMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ id }: { id: string }) => {
      await resources.deleteApp(id);
      return id;
    },
    onSuccess: (id) => {
      queryClient.removeQueries({ queryKey: queryKeys.app(scope, id) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.apps(scope) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.deployments(scope) });
    },
  });
}

export function useCreateDeploymentMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      appId,
      source,
      entrypoint,
    }: {
      appId: string;
      source: File;
      entrypoint: string;
    }) => (await resources.createDeployment(appId, source, entrypoint)).data,
    onSuccess: (deployment) => {
      queryClient.setQueryData(queryKeys.deployment(scope, deployment.id), deployment);
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "deployments"] });
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "builds"] });
    },
  });
}

export function useCreateExecutionMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ deploymentId, input }: { deploymentId: string; input: unknown }) =>
      (await resources.createExecution(deploymentId, input)).data,
    onSuccess: (execution) => {
      queryClient.setQueryData(queryKeys.execution(scope, execution.id), execution);
      void queryClient.invalidateQueries({
        queryKey: queryKeys.executions(scope, execution.deployment_id),
      });
      void queryClient.invalidateQueries({ queryKey: queryKeys.executions(scope) });
    },
  });
}

export function useRerunExecutionMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => (await resources.rerunExecution(id)).data,
    onSuccess: (execution) => {
      queryClient.setQueryData(queryKeys.execution(scope, execution.id), execution);
      void queryClient.invalidateQueries({ queryKey: queryKeys.executions(scope) });
    },
  });
}

export function useUpdateUserMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      body,
    }: {
      id: string;
      body: { email?: string; disabled?: boolean };
    }) => (await resources.updateUser(id, body)).data,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.users(scope) }),
  });
}

export function useCreateAPIKeyMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: { name: string; user_id?: string; scopes: string[] }) =>
      (await resources.createAPIKey(body)).data,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(scope) }),
  });
}

export function useRevokeAPIKeyMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => (await resources.revokeAPIKey(id)).data,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.apiKeys(scope) }),
  });
}

/* -------------------------------------------------------------------------- */
/* Browser profiles                                                            */
/* -------------------------------------------------------------------------- */

export function useBrowserProfilesQuery() {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.browserProfiles(scope),
    queryFn: async ({ signal }) => (await resources.listBrowserProfiles(signal)).data,
  });
}

/**
 * Fetches a profile's cookie values and storage contents.
 *
 * Disabled by default: this is the one response that carries credentials, so it
 * is fetched when somebody asks to see them, not on every render of the list.
 */
export function useBrowserProfileStateQuery(id: string | null) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.browserProfileState(scope, id ?? ""),
    queryFn: async ({ signal }) =>
      (await resources.getBrowserProfileState(id as string, signal)).data,
    enabled: id !== null,
    gcTime: 0,
  });
}

export function useCreateBrowserProfileMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: Parameters<typeof resources.createBrowserProfile>[0]) =>
      (await resources.createBrowserProfile(body)).data,
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.browserProfiles(scope) }),
  });
}

export function useUpdateBrowserProfileMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({
      id,
      ...body
    }: { id: string } & Parameters<typeof resources.updateBrowserProfile>[1]) =>
      (await resources.updateBrowserProfile(id, body)).data,
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.browserProfiles(scope) }),
  });
}

export function useDeleteBrowserProfileMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (id: string) => {
      await resources.deleteBrowserProfile(id);
    },
    onSuccess: () =>
      void queryClient.invalidateQueries({ queryKey: queryKeys.browserProfiles(scope) }),
  });
}

