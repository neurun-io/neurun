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
import { NeurunApiError } from "./errors";
import { LIVE_POLL_INTERVAL_MS } from "./query-client";
import { isTerminalInvocationStatus, isTerminalJobState } from "./types";
import type { CreateJobRequest, FetchRequest, InvokeFunctionRequest, Job, JobState } from "./types";

/* -------------------------------------------------------------------------- */
/* Keys                                                                        */
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
