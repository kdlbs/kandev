import type { TFunction } from "i18next";
import { ApiError } from "@/lib/api/client";

/** Converts the structured conflict into localized copy without showing raw server errors. */
export function secretDeleteErrorMessage(error: unknown, t: TFunction): string {
  if (!(error instanceof ApiError) || error.status !== 409) return t("settings:secretDeleteFailed");
  const body = error.body;
  if (!body || typeof body !== "object" || !("code" in body) || body.code !== "secret_in_use") {
    return t("settings:secretDeleteFailed");
  }
  const refs = "references" in body && Array.isArray(body.references) ? body.references : [];
  const labels = refs.map((ref: unknown) => referenceLabel(ref, t)).filter(Boolean);
  return labels.length
    ? t("settings:secretInUse", { references: labels.join(", ") })
    : t("settings:secretInUseUnknown");
}

function referenceLabel(value: unknown, t: TFunction): string | null {
  if (!value || typeof value !== "object") return null;
  const ref = value as Record<string, unknown>;
  const name = typeof ref.name === "string" ? ref.name : "";
  const key = typeof ref.key === "string" ? ref.key : "";
  if (ref.kind === "repository" && !name && !key) return t("settings:secretReferenceHidden");
  if (!name || !key) return null;
  switch (ref.kind) {
    case "agent_profile":
      return t("settings:secretReferenceAgent", { name, key });
    case "executor_profile":
      return t("settings:secretReferenceExecutor", { name, key });
    case "repository":
      return t("settings:secretReferenceRepository", { name, key });
    default:
      return null;
  }
}
