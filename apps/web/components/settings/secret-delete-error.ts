import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";
import type { SecretReference } from "@/lib/types/http-secrets";

export function secretReferencesFromError(error: unknown): SecretReference[] | null {
  if (!(error instanceof ApiError) || error.status !== 409) return null;
  const body = error.body;
  if (!body || typeof body !== "object" || !("code" in body) || body.code !== "secret_in_use") {
    return null;
  }
  if (!("references" in body) || !Array.isArray(body.references)) return null;
  return body.references.filter(isSecretReference);
}

/** Converts the structured conflict into localized copy without showing raw server errors. */
export function secretDeleteErrorMessage(error: unknown, t: TFunction): string {
  if (!isSecretInUseError(error)) return t("settings:secretDeleteFailed");
  const refs = secretReferencesFromError(error) ?? [];
  const labels = refs.map((ref) => secretReferenceLabel(ref, t));
  return labels.length
    ? t("settings:secretInUse", { references: labels.join(", ") })
    : t("settings:secretInUseUnknown");
}

export function secretReferenceLabel(ref: SecretReference, t: TFunction): string {
  const name = ref.name ?? "";
  const key = ref.key ?? "";
  if (!name && !key) return t("settings:secretReferenceHidden");
  if (!name || !key) return t("settings:secretReferenceHidden");
  switch (ref.kind) {
    case "agent_profile":
      return t("settings:secretReferenceAgent", { name, key });
    case "executor_profile":
      return t("settings:secretReferenceExecutor", { name, key });
    case "repository":
      return t("settings:secretReferenceRepository", { name, key });
    default:
      return t("settings:secretReferenceHidden");
  }
}

function isSecretReference(value: unknown): value is SecretReference {
  if (!value || typeof value !== "object") return false;
  const ref = value as Record<string, unknown>;
  return (
    ref.kind === "agent_profile" || ref.kind === "executor_profile" || ref.kind === "repository"
  );
}

function isSecretInUseError(error: unknown): error is ApiError {
  const body = error instanceof ApiError ? error.body : null;
  return (
    error instanceof ApiError &&
    error.status === 409 &&
    Boolean(body && typeof body === "object" && "code" in body && body.code === "secret_in_use")
  );
}
