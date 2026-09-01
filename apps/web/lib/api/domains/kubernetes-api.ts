import { fetchJson, type ApiRequestOptions } from "../client";
import type {
  KubernetesSession,
  KubernetesSessionImpact,
  KubernetesTestRequest,
  KubernetesTestResult,
  KubernetesTestStep,
  KubernetesWarning,
} from "@/lib/types/http-kubernetes";

export function normalizeKubernetesTestResult(value: unknown): KubernetesTestResult {
  const record = asRecord(value);
  const result: KubernetesTestResult = {
    success: record?.success === true,
    steps: Array.isArray(record?.steps) ? record.steps.flatMap(normalizeTestStep) : [],
    warnings: Array.isArray(record?.warnings) ? record.warnings.flatMap(normalizeWarning) : [],
  };
  assignOptionalString(result, "server_version", record?.server_version);
  assignOptionalString(result, "namespace", record?.namespace);
  assignOptionalString(result, "error", record?.error);
  return result;
}

export function normalizeKubernetesSessions(value: unknown): KubernetesSession[] {
  if (!Array.isArray(value)) return [];
  return value.flatMap(normalizeSession);
}

export async function testKubernetesConnection(
  request: KubernetesTestRequest,
  options?: ApiRequestOptions,
): Promise<KubernetesTestResult> {
  const headers = new Headers(options?.init?.headers);
  headers.set("Content-Type", "application/json");
  const response = await fetchJson<unknown>("/api/v1/kubernetes/test", {
    ...options,
    init: {
      ...(options?.init ?? {}),
      method: "POST",
      headers,
      body: JSON.stringify(request),
    },
  });
  return normalizeKubernetesTestResult(response);
}

export async function listKubernetesSessions(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<KubernetesSession[]> {
  const response = await fetchJson<unknown>(
    `/api/v1/kubernetes/executors/${encodeURIComponent(executorId)}/sessions`,
    options,
  );
  return normalizeKubernetesSessions(response);
}

export async function getKubernetesTaskSession(
  executorId: string,
  taskId: string,
  sessionId: string,
  options?: ApiRequestOptions,
): Promise<KubernetesSession | null> {
  const query = new URLSearchParams({ task_id: taskId, session_id: sessionId });
  const response = await fetchJson<unknown>(
    `/api/v1/kubernetes/executors/${encodeURIComponent(executorId)}/sessions?${query}`,
    options,
  );
  return (
    normalizeKubernetesSessions(response).find(
      (session) => session.task_id === taskId && session.session_id === sessionId,
    ) ?? null
  );
}

export async function getKubernetesSessionImpact(
  executorId: string,
  options?: ApiRequestOptions,
): Promise<KubernetesSessionImpact> {
  return fetchJson<KubernetesSessionImpact>(
    `/api/v1/kubernetes/executors/${encodeURIComponent(executorId)}/session-impact`,
    options,
  );
}

function normalizeTestStep(value: unknown): KubernetesTestStep[] {
  const record = asRecord(value);
  if (!record || typeof record.key !== "string" || typeof record.detail !== "string") return [];
  const step: KubernetesTestStep = {
    key: record.key,
    success: record.success === true,
    duration_ms: finiteNumber(record.duration_ms),
    detail: record.detail,
  };
  assignOptionalString(step, "error", record.error);
  return [step];
}

function normalizeWarning(value: unknown): KubernetesWarning[] {
  const record = asRecord(value);
  if (!record || typeof record.path !== "string" || typeof record.message !== "string") return [];
  return [{ path: record.path, message: record.message }];
}

function normalizeSession(value: unknown): KubernetesSession[] {
  const record = asRecord(value);
  if (!record || typeof record.session_id !== "string" || typeof record.task_id !== "string") {
    return [];
  }
  const session: KubernetesSession = {
    session_id: record.session_id,
    task_id: record.task_id,
    restarts: finiteNumber(record.restarts),
  };
  for (const key of SESSION_STRING_KEYS) assignOptionalString(session, key, record[key]);
  return [session];
}

const SESSION_STRING_KEYS = [
  "pod_name",
  "pod_phase",
  "container_state",
  "workspace_kind",
  "created_at",
  "failure_reason",
] as const;

function asRecord(value: unknown): Record<string, unknown> | null {
  return value !== null && typeof value === "object" ? (value as Record<string, unknown>) : null;
}

function finiteNumber(value: unknown): number {
  return typeof value === "number" && Number.isFinite(value) ? value : 0;
}

function assignOptionalString<T extends object, K extends keyof T>(
  target: T,
  key: K,
  value: unknown,
): void {
  if (typeof value === "string" && value) target[key] = value as T[K];
}
