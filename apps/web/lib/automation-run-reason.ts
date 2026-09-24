import type { TFunction } from "i18next";
import type { AutomationRun } from "@/lib/types/automation";

/**
 * Stored disposition tokens are backend-owned identifiers (internal/automation
 * DedupReason / internal/orchestrator RepositoryReason), not display copy —
 * this maps each one to a translated label. `dedup_not_configured` has no
 * entry: it is excluded from rendering by the caller, not translated.
 */
const REASON_KEYS: Record<string, string> = {
  dedup_unresolved: "automations:runReasonDedupUnresolved",
  selector_unresolved: "automations:runReasonSelectorUnresolved",
  repository_none_configured: "automations:runReasonRepositoryNoneConfigured",
  repository_load_failed: "automations:runReasonRepositoryLoadFailed",
  repository_continuation_reused: "automations:runReasonRepositoryContinuationReused",
  selector_no_match: "automations:runReasonSelectorNoMatch",
  selector_ambiguous: "automations:runReasonSelectorAmbiguous",
};

// Two tokens carry a value appended as "<token>: <value>"; every other token
// is bare.
function splitReasonToken(raw: string): { token: string; value?: string } {
  const separatorIndex = raw.indexOf(": ");
  if (separatorIndex === -1) return { token: raw };
  return { token: raw.slice(0, separatorIndex), value: raw.slice(separatorIndex + 2) };
}

function formatReasonToken(t: TFunction, raw: string): string {
  const { token, value } = splitReasonToken(raw);
  const key = REASON_KEYS[token];
  if (!key) return raw;
  return value !== undefined ? t(key, { value }) : t(key);
}

/**
 * Builds the muted reason suffix for a run's outcome cell: repository reason
 * first, then dedup reason, excluding the non-operator-facing
 * "dedup_not_configured" token. Empty when neither reason renders.
 */
export function buildRunOutcomeReasonSuffix(t: TFunction, run: AutomationRun): string {
  return [run.repository_reason, run.dedup_reason]
    .filter((reason): reason is string => !!reason && reason !== "dedup_not_configured")
    .map((reason) => formatReasonToken(t, reason))
    .join(" · ");
}
