"use client";

/**
 * Query and mutation hooks.
 *
 * Every key is prefixed with the session scope (project + operator ID) so cached
 * evidence can never bleed between operators. Mutations invalidate explicitly —
 * nothing here relies on a blanket refetch to eventually become correct.
 */
import { useMutation, useQuery, useQueryClient, type UseQueryOptions } from "@tanstack/react-query";
import { sessionScope, useSession } from "@/lib/session/store";
import { useCapability } from "@/lib/session/capability";
import * as api from "./endpoints";
import * as resources from "./resources";
import { NeurunApiError } from "./errors";
import { LIVE_POLL_INTERVAL_MS } from "./query-client";
import { isTerminalInvocationStatus, isTerminalJobState } from "./types";
import { isTerminalExecutionStatus } from "./resource-types";
import type { CreateJobRequest, FetchRequest, InvokeFunctionRequest, Job, JobState } from "./types";
import type { UserRole } from "./resource-types";

/* -------------------------------------------------------------------------- */
/* Query keys                                                                  */
/* -------------------------------------------------------------------------- */

/**
 * Query keys are built from the session scope rather than a credential, so
 * nothing in a cache key can ever contain a secret.
 */
export function useScope(): string {
  const { operator } = useSession();
  return sessionScope(operator);
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

  functions: (scope: string, params?: api.ListFunctionsParams) =>
    [...queryKeys.scope(scope), "functions", params ?? {}] as const,
  function: (scope: string, name: string) => [...queryKeys.scope(scope), "function", name] as const,
  functionVersion: (scope: string, name: string, version: string) =>
    [...queryKeys.scope(scope), "function", name, "version", version] as const,

  jobs: (scope: string, params?: api.ListJobsParams) =>
    [...queryKeys.scope(scope), "jobs", params ?? {}] as const,
  job: (scope: string, jobId: string) => [...queryKeys.scope(scope), "job", jobId] as const,
  jobEvents: (scope: string, jobId: string) =>
    [...queryKeys.scope(scope), "job", jobId, "events"] as const,
  jobAttempts: (scope: string, jobId: string) =>
    [...queryKeys.scope(scope), "job", jobId, "attempts"] as const,

  invocations: (scope: string, params?: api.ListInvocationsParams) =>
    [...queryKeys.scope(scope), "invocations", params ?? {}] as const,
  invocation: (scope: string, id: string) =>
    [...queryKeys.scope(scope), "invocation", id] as const,
} as const;

/* -------------------------------------------------------------------------- */
/* Health                                                                      */
/* -------------------------------------------------------------------------- */

export function useVersionQuery(options?: { enabled?: boolean }) {
  const scope = useScope();
  const { status } = useSession();

  return useQuery({
    queryKey: queryKeys.version(scope),
    queryFn: async ({ signal }) => (await api.getVersion(signal)).data,
    enabled: status === "authenticated" && (options?.enabled ?? true),
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

export function useCreateAppMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name }: { name: string }) => (await resources.createApp(name)).data,
    onSuccess: (app) => {
      queryClient.setQueryData(queryKeys.app(scope, app.id), app);
      void queryClient.invalidateQueries({ queryKey: queryKeys.apps(scope) });
    },
  });
}

export function useUpdateAppMutation(id: string) {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async ({ name }: { name: string }) => (await resources.updateApp(id, name)).data,
    onSuccess: (app) => {
      queryClient.setQueryData(queryKeys.app(scope, id), app);
      void queryClient.invalidateQueries({ queryKey: queryKeys.apps(scope) });
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

export function useCreateUserMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (body: {
      username: string;
      display_name: string;
      role: UserRole;
      password: string;
    }) => (await resources.createUser(body)).data,
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.users(scope) }),
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
      body: { display_name?: string; role?: UserRole; disabled?: boolean };
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
/* Functions                                                                   */
/* -------------------------------------------------------------------------- */

export function useFunctionsQuery(params: api.ListFunctionsParams = {}) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.functions(scope, params),
    queryFn: async ({ signal }) => (await api.listFunctions(params, signal)).data,
    // The catalog is release-owned and immutable for a given server build.
    staleTime: 5 * 60_000,
  });
}

export function useFunctionQuery(name: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.function(scope, name),
    queryFn: async ({ signal }) => (await api.getFunction(name, signal)).data,
    staleTime: 5 * 60_000,
    enabled: Boolean(name),
  });
}

export function useFunctionVersionQuery(name: string, version: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.functionVersion(scope, name, version),
    queryFn: async ({ signal }) => (await api.getFunctionVersion(name, version, signal)).data,
    staleTime: 5 * 60_000,
    enabled: Boolean(name && version),
  });
}

/* -------------------------------------------------------------------------- */
/* Jobs                                                                        */
/* -------------------------------------------------------------------------- */

export function useJobsQuery(params: api.ListJobsParams = {}, options?: { live?: boolean }) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.jobs(scope, params),
    queryFn: async ({ signal }) => (await api.listJobs(params, signal)).data,
    // Keep the previous page on screen while the next one loads, so the table
    // does not collapse to a spinner on every cursor step.
    placeholderData: (previous) => previous,
    refetchInterval: options?.live === false ? false : LIVE_POLL_INTERVAL_MS,
  });
}

/**
 * A job snapshot. Polls about every two seconds while the job is non-terminal
 * and stops the moment it settles — there is no SSE endpoint in the current
 * contract, and a terminal job cannot change again.
 */
export function useJobQuery(jobId: string, options?: Partial<UseQueryOptions<Job>>) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.job(scope, jobId),
    queryFn: async ({ signal }) => (await api.getJob(jobId, signal)).data,
    enabled: Boolean(jobId),
    refetchInterval: (query) => {
      const state = query.state.data?.state;
      if (!state) return LIVE_POLL_INTERVAL_MS;
      return isTerminalJobState(state) ? false : LIVE_POLL_INTERVAL_MS;
    },
    ...options,
  });
}

export function useJobEventsQuery(jobId: string, live: boolean) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.jobEvents(scope, jobId),
    queryFn: async ({ signal }) => (await api.listJobEvents(jobId, signal)).data,
    enabled: Boolean(jobId),
    refetchInterval: live ? LIVE_POLL_INTERVAL_MS : false,
  });
}

export function useJobAttemptsQuery(jobId: string, live: boolean) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.jobAttempts(scope, jobId),
    queryFn: async ({ signal }) => (await api.listJobAttempts(jobId, signal)).data,
    enabled: Boolean(jobId),
    refetchInterval: live ? LIVE_POLL_INTERVAL_MS : false,
  });
}

/* -------------------------------------------------------------------------- */
/* Invocations                                                                 */
/* -------------------------------------------------------------------------- */

export function useInvocationsQuery(params: api.ListInvocationsParams = {}) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.invocations(scope, params),
    queryFn: async ({ signal }) => (await api.listFunctionInvocations(params, signal)).data,
    placeholderData: (previous) => previous,
    refetchInterval: LIVE_POLL_INTERVAL_MS,
  });
}

export function useInvocationQuery(invocationId: string) {
  const scope = useScope();
  return useQuery({
    queryKey: queryKeys.invocation(scope, invocationId),
    queryFn: async ({ signal }) => (await api.getFunctionInvocation(invocationId, signal)).data,
    enabled: Boolean(invocationId),
    refetchInterval: (query) => {
      const status = query.state.data?.status;
      if (!status) return LIVE_POLL_INTERVAL_MS;
      return isTerminalInvocationStatus(status) ? false : LIVE_POLL_INTERVAL_MS;
    },
  });
}

/* -------------------------------------------------------------------------- */
/* Mutations                                                                   */
/* -------------------------------------------------------------------------- */

export function useCancelJobMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ jobId, reason }: { jobId: string; reason?: string }) =>
      (await api.cancelJob(jobId, reason)).data,
    onSuccess: (data, { jobId }) => {
      queryClient.setQueryData(queryKeys.job(scope, jobId), data.job);
      void queryClient.invalidateQueries({ queryKey: queryKeys.jobEvents(scope, jobId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.jobAttempts(scope, jobId) });
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "jobs"] });
    },
  });
}

export function useCancelInvocationMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (invocationId: string) =>
      (await api.cancelFunctionInvocation(invocationId)).data,
    onSuccess: (_data, invocationId) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.invocation(scope, invocationId) });
      void queryClient.invalidateQueries({
        queryKey: [...queryKeys.scope(scope), "invocations"],
      });
    },
  });
}

/**
 * Note the shared shape of the three mutation hooks below: every accepted
 * asynchronous mutation records the reported `durability`, and every
 * `durable_backend_unavailable` failure records that async is off. Doing this
 * inside the hooks rather than at each call site means the connection-level
 * warning and the async gating hold no matter which surface submitted the work.
 */
export function useCreateJobMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async (body: CreateJobRequest) => await api.createJob(body),
    onSuccess: ({ data, meta }) => {
      recordDurability(data.durability ?? meta.durability);
      queryClient.setQueryData(queryKeys.job(scope, data.job_id), data.job);
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "jobs"] });
    },
    onError: (error) => {
      if (error instanceof NeurunApiError && error.isDurableBackendUnavailable) {
        recordAsyncUnavailable();
      }
    },
  });
}

export function useInvokeFunctionMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async ({
      functionName,
      body,
    }: {
      functionName: string;
      body: InvokeFunctionRequest;
    }) => await api.invokeFunction(functionName, body),
    onSuccess: (outcome) => {
      if (outcome.kind === "accepted") {
        const { data, meta } = outcome.result;
        recordDurability(data.durability ?? meta.durability);
        queryClient.setQueryData(queryKeys.job(scope, data.job_id), data.job);
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "jobs"] });
      } else {
        void queryClient.invalidateQueries({
          queryKey: [...queryKeys.scope(scope), "invocations"],
        });
      }
    },
    onError: (error) => {
      if (error instanceof NeurunApiError && error.isDurableBackendUnavailable) {
        recordAsyncUnavailable();
      }
    },
  });
}

export function useFetchMutation() {
  const scope = useScope();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async (body: FetchRequest) => await api.fetchUrl(body),
    onSuccess: (outcome) => {
      if (outcome.kind === "accepted") {
        const { data, meta } = outcome.result;
        recordDurability(data.durability ?? meta.durability);
        queryClient.setQueryData(queryKeys.job(scope, data.job_id), data.job);
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(scope), "jobs"] });
      } else {
        void queryClient.invalidateQueries({
          queryKey: [...queryKeys.scope(scope), "invocations"],
        });
      }
    },
    onError: (error) => {
      if (error instanceof NeurunApiError && error.isDurableBackendUnavailable) {
        recordAsyncUnavailable();
      }
    },
  });
}

export type { JobState };
