import { KubernetesExecutorRollbackRejectedError } from "@/hooks/domains/settings/use-kubernetes-settings";

type Translate = (key: string, values?: Record<string, unknown>) => string;

export function kubernetesCreateErrorMessage(error: unknown, t: Translate): string {
  const createFallback = t("executors:kubernetesCreateFailed");
  if (!(error instanceof AggregateError)) return readableError(error, createFallback);
  const errors = error.errors as unknown[];
  return t("executors:kubernetesCreateRollbackFailed", {
    profileError: readableError(errors[0], createFallback),
    rollbackError: rollbackErrorMessage(errors[1], t),
  });
}

function rollbackErrorMessage(error: unknown, t: Translate): string {
  if (error instanceof KubernetesExecutorRollbackRejectedError) {
    return t("executors:kubernetesCreateRollbackRejected");
  }
  return readableError(error, t("executors:kubernetesCreateRollbackUnknown"));
}

function readableError(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}
