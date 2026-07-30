"use client";

/**
 * Query and mutation hooks.
 *
 * Every key is prefixed with the connection scope (base URL + non-secret key
 * identity) so cached evidence can never bleed across projects or control
 * planes. Mutations invalidate explicitly — nothing here relies on a blanket
 * refetch to eventually become correct.
 */
import {
  useMutation,
  useQuery,
  useQueryClient,
  type UseQueryOptions,
} from "@tanstack/react-query";
import { connectionScope, useConnection, useRequiredConnection } from "@/lib/connection/store";
import { useCapability } from "@/lib/connection/capability";
import type { Connection } from "./client";
import * as api from "./endpoints";
import { NeurunApiError } from "./errors";
import { LIVE_POLL_INTERVAL_MS } from "./query-client";
import { isTerminalInvocationStatus, isTerminalJobState } from "./types";
import type {
  CreateJobRequest,
  FetchRequest,
  InvokeFunctionRequest,
  Job,
  JobState,
} from "./types";

/* -------------------------------------------------------------------------- */
/* Keys                                                                        */
/* -------------------------------------------------------------------------- */

export const queryKeys = {
  scope: (connection: Connection | null) => ["neurun", connectionScope(connection)] as const,

  version: (c: Connection | null) => [...queryKeys.scope(c), "version"] as const,

  functions: (c: Connection | null, params?: api.ListFunctionsParams) =>
    [...queryKeys.scope(c), "functions", params ?? {}] as const,
  function: (c: Connection | null, name: string) =>
    [...queryKeys.scope(c), "function", name] as const,
  functionVersion: (c: Connection | null, name: string, version: string) =>
    [...queryKeys.scope(c), "function", name, "version", version] as const,

  jobs: (c: Connection | null, params?: api.ListJobsParams) =>
    [...queryKeys.scope(c), "jobs", params ?? {}] as const,
  job: (c: Connection | null, jobId: string) => [...queryKeys.scope(c), "job", jobId] as const,
  jobEvents: (c: Connection | null, jobId: string) =>
    [...queryKeys.scope(c), "job", jobId, "events"] as const,
  jobAttempts: (c: Connection | null, jobId: string) =>
    [...queryKeys.scope(c), "job", jobId, "attempts"] as const,

  invocations: (c: Connection | null, params?: api.ListInvocationsParams) =>
    [...queryKeys.scope(c), "invocations", params ?? {}] as const,
  invocation: (c: Connection | null, id: string) =>
    [...queryKeys.scope(c), "invocation", id] as const,
} as const;

/* -------------------------------------------------------------------------- */
/* Health                                                                      */
/* -------------------------------------------------------------------------- */

export function useVersionQuery(options?: { enabled?: boolean }) {
  const { connection } = useConnection();
  return useQuery({
    queryKey: queryKeys.version(connection),
    queryFn: async ({ signal }) => (await api.getVersion(connection!, signal)).data,
    enabled: Boolean(connection) && (options?.enabled ?? true),
    staleTime: 60_000,
    refetchInterval: 60_000,
  });
}

/* -------------------------------------------------------------------------- */
/* Functions                                                                   */
/* -------------------------------------------------------------------------- */

export function useFunctionsQuery(params: api.ListFunctionsParams = {}) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.functions(connection, params),
    queryFn: async ({ signal }) => (await api.listFunctions(connection, params, signal)).data,
    // The catalog is release-owned and immutable for a given server build.
    staleTime: 5 * 60_000,
  });
}

export function useFunctionQuery(name: string) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.function(connection, name),
    queryFn: async ({ signal }) => (await api.getFunction(connection, name, signal)).data,
    staleTime: 5 * 60_000,
    enabled: Boolean(name),
  });
}

export function useFunctionVersionQuery(name: string, version: string) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.functionVersion(connection, name, version),
    queryFn: async ({ signal }) =>
      (await api.getFunctionVersion(connection, name, version, signal)).data,
    staleTime: 5 * 60_000,
    enabled: Boolean(name && version),
  });
}

/* -------------------------------------------------------------------------- */
/* Jobs                                                                        */
/* -------------------------------------------------------------------------- */

export function useJobsQuery(params: api.ListJobsParams = {}, options?: { live?: boolean }) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.jobs(connection, params),
    queryFn: async ({ signal }) => (await api.listJobs(connection, params, signal)).data,
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
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.job(connection, jobId),
    queryFn: async ({ signal }) => (await api.getJob(connection, jobId, signal)).data,
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
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.jobEvents(connection, jobId),
    queryFn: async ({ signal }) => (await api.listJobEvents(connection, jobId, signal)).data,
    enabled: Boolean(jobId),
    refetchInterval: live ? LIVE_POLL_INTERVAL_MS : false,
  });
}

export function useJobAttemptsQuery(jobId: string, live: boolean) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.jobAttempts(connection, jobId),
    queryFn: async ({ signal }) => (await api.listJobAttempts(connection, jobId, signal)).data,
    enabled: Boolean(jobId),
    refetchInterval: live ? LIVE_POLL_INTERVAL_MS : false,
  });
}

/* -------------------------------------------------------------------------- */
/* Invocations                                                                 */
/* -------------------------------------------------------------------------- */

export function useInvocationsQuery(params: api.ListInvocationsParams = {}) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.invocations(connection, params),
    queryFn: async ({ signal }) =>
      (await api.listFunctionInvocations(connection, params, signal)).data,
    placeholderData: (previous) => previous,
    refetchInterval: LIVE_POLL_INTERVAL_MS,
  });
}

export function useInvocationQuery(invocationId: string) {
  const connection = useRequiredConnection();
  return useQuery({
    queryKey: queryKeys.invocation(connection, invocationId),
    queryFn: async ({ signal }) =>
      (await api.getFunctionInvocation(connection, invocationId, signal)).data,
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
  const connection = useRequiredConnection();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async ({ jobId, reason }: { jobId: string; reason?: string }) =>
      (await api.cancelJob(connection, jobId, reason)).data,
    onSuccess: (data, { jobId }) => {
      queryClient.setQueryData(queryKeys.job(connection, jobId), data.job);
      void queryClient.invalidateQueries({ queryKey: queryKeys.jobEvents(connection, jobId) });
      void queryClient.invalidateQueries({ queryKey: queryKeys.jobAttempts(connection, jobId) });
      void queryClient.invalidateQueries({
        queryKey: [...queryKeys.scope(connection), "jobs"],
      });
    },
  });
}

export function useCancelInvocationMutation() {
  const connection = useRequiredConnection();
  const queryClient = useQueryClient();

  return useMutation({
    mutationFn: async (invocationId: string) =>
      (await api.cancelFunctionInvocation(connection, invocationId)).data,
    onSuccess: (_data, invocationId) => {
      void queryClient.invalidateQueries({
        queryKey: queryKeys.invocation(connection, invocationId),
      });
      void queryClient.invalidateQueries({
        queryKey: [...queryKeys.scope(connection), "invocations"],
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
  const connection = useRequiredConnection();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async (body: CreateJobRequest) => await api.createJob(connection, body),
    onSuccess: ({ data, meta }) => {
      recordDurability(data.durability ?? meta.durability);
      queryClient.setQueryData(queryKeys.job(connection, data.job_id), data.job);
      void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(connection), "jobs"] });
    },
    onError: (error) => {
      if (error instanceof NeurunApiError && error.isDurableBackendUnavailable) {
        recordAsyncUnavailable();
      }
    },
  });
}

export function useInvokeFunctionMutation() {
  const connection = useRequiredConnection();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async ({
      functionName,
      body,
    }: {
      functionName: string;
      body: InvokeFunctionRequest;
    }) => await api.invokeFunction(connection, functionName, body),
    onSuccess: (outcome) => {
      if (outcome.kind === "accepted") {
        const { data, meta } = outcome.result;
        recordDurability(data.durability ?? meta.durability);
        queryClient.setQueryData(queryKeys.job(connection, data.job_id), data.job);
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(connection), "jobs"] });
      } else {
        void queryClient.invalidateQueries({
          queryKey: [...queryKeys.scope(connection), "invocations"],
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
  const connection = useRequiredConnection();
  const queryClient = useQueryClient();
  const { recordDurability, recordAsyncUnavailable } = useCapability();

  return useMutation({
    mutationFn: async (body: FetchRequest) => await api.fetchUrl(connection, body),
    onSuccess: (outcome) => {
      if (outcome.kind === "accepted") {
        const { data, meta } = outcome.result;
        recordDurability(data.durability ?? meta.durability);
        queryClient.setQueryData(queryKeys.job(connection, data.job_id), data.job);
        void queryClient.invalidateQueries({ queryKey: [...queryKeys.scope(connection), "jobs"] });
      } else {
        void queryClient.invalidateQueries({
          queryKey: [...queryKeys.scope(connection), "invocations"],
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
