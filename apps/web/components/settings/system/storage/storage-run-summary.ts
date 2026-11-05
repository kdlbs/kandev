import type { StorageMaintenanceRun, StorageRunState } from "@/lib/types/system";

export type StorageRunDisplayStatus =
  | "queued"
  | "running"
  | "succeeded"
  | "failed"
  | "cancelled"
  | "skipped_busy"
  | "incomplete"
  | "partial"
  | "unknown";

export type StorageHistoryIssueCategory =
  | "entries_changed"
  | "entries_preserved"
  | "unreadable"
  | "delete_failed"
  | "boundary_preserved"
  | "entry_limit"
  | "deadline"
  | "cancelled"
  | "other"
  | "unknown";

export interface StorageGoCacheOutcome {
  reclaimedBytes: number;
  bytesAfter: number | null;
  skipped: boolean;
  partial: boolean;
  issueCategories: StorageHistoryIssueCategory[];
  hasUnknownIssue: boolean;
  otherProvidersSkipped: boolean;
}

export interface StorageRunSummary {
  status: StorageRunDisplayStatus;
  goCache: StorageGoCacheOutcome | null;
  hasProviderError: boolean;
  hasNonGoProviderError: boolean;
  otherProvidersSkipped: boolean;
  busyPolicyEnabled: boolean;
}

const RESERVED_RESULT_KEYS = new Set(["skipped_providers", "busy_resources"]);
const USEFUL_BYTE_FIELDS = ["reclaimed_bytes", "quarantined_bytes", "deleted_bytes"] as const;
const PRESERVED_STATES: readonly StorageRunState[] = [
  "queued",
  "running",
  "cancelled",
  "skipped_busy",
];
const KNOWN_STATES = new Set<string>([...PRESERVED_STATES, "succeeded", "failed"]);
const EXPECTED_PROGRESS_REASONS = new Set(["incomplete_scan", "cleanup_in_progress"]);
const BUDGET_ISSUE = "entry_limit_reached";

const ISSUE_CATEGORY_BY_CODE: Readonly<Record<string, StorageHistoryIssueCategory>> = {
  cache_parent_changed: "entries_changed",
  path_changed_or_unsafe: "entries_changed",
  entry_read_failed: "unreadable",
  entry_delete_failed: "delete_failed",
  symlink_skipped: "entries_preserved",
  unsupported_entry_skipped: "entries_preserved",
  directory_depth_limit_reached: "entries_preserved",
  filesystem_boundary_skipped: "boundary_preserved",
  cache_root_changed: "boundary_preserved",
  directory_read_failed: "unreadable",
  directory_open_failed: "unreadable",
  entry_limit_reached: "entry_limit",
  cleanup_deadline_reached: "deadline",
  cleanup_cancelled: "cancelled",
};

// A recognized failure category already explains a raw cause. Progress and
// stop categories do not, so uncovered raw errors stay visible beside them.
const FAILURE_CATEGORIES: ReadonlySet<StorageHistoryIssueCategory> = new Set([
  "entries_changed",
  "entries_preserved",
  "unreadable",
  "delete_failed",
  "boundary_preserved",
  "unknown",
]);

function recordValue(value: unknown): Record<string, unknown> | undefined {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : undefined;
}

function runResultRecord(result: Record<string, unknown>): Record<string, unknown> {
  return recordValue(result.result) ?? result;
}

function finiteNonNegative(value: unknown): number | null {
  return typeof value === "number" && Number.isFinite(value) && value >= 0 ? value : null;
}

function providerEntries(result: Record<string, unknown>): Array<[string, unknown]> {
  return Object.entries(runResultRecord(result)).filter(
    ([name]) => !RESERVED_RESULT_KEYS.has(name),
  );
}

function providerHasFailure(entry: unknown): boolean {
  const provider = recordValue(entry);
  if (!provider) return true;
  const error = provider.error;
  if (error !== undefined && error !== null) {
    if (typeof error !== "string" || error.trim() !== "") return true;
  }
  const result = provider.result;
  if (result === undefined || result === null) return false;
  const record = recordValue(result);
  if (!record) return true;
  if (record.partial === true) return true;
  const issues = record.errors;
  if (issues !== undefined) {
    if (!Array.isArray(issues) || issues.some((issue) => typeof issue !== "string")) return true;
    if (issues.length > 0) return true;
  }
  const failedCount = record.failed;
  return typeof failedCount === "number" && failedCount > 0;
}

function budgetOnlyIssues(value: unknown): boolean {
  return Array.isArray(value) && value.length > 0 && value.every((issue) => issue === BUDGET_ISSUE);
}

function budgetOnlyText(value: string): boolean {
  return rawErrorTokens(value).every((token) => token === BUDGET_ISSUE);
}

function budgetOnlyError(value: unknown): boolean {
  if (value === undefined || value === null) return true;
  return typeof value === "string" && budgetOnlyText(value);
}

function expectedGoCacheProgress(
  providers: Array<[string, unknown]>,
  message: string | undefined,
): boolean {
  const goCache = recordValue(providers.find(([name]) => name === "go_cache")?.[1]);
  const cleanup = recordValue(goCache?.result);
  if (!goCache || !cleanup) return false;
  if (typeof cleanup.reason !== "string" || !EXPECTED_PROGRESS_REASONS.has(cleanup.reason)) {
    return false;
  }
  if (!budgetOnlyIssues(cleanup.errors) || !budgetOnlyError(goCache.error)) return false;
  if (message !== undefined && !budgetOnlyError(message)) return false;
  return providers.every(([name, entry]) => name === "go_cache" || !providerHasFailure(entry));
}

function providerUsefulBytes(entry: unknown): number {
  const result = recordValue(recordValue(entry)?.result);
  if (!result) return 0;
  return USEFUL_BYTE_FIELDS.reduce((total, field) => {
    const value = finiteNonNegative(result[field]);
    return value !== null ? total + value : total;
  }, 0);
}

function hasUsefulWork(
  providers: Array<[string, unknown]>,
  goCache: StorageGoCacheOutcome | null,
): boolean {
  if (goCache !== null && goCache.reclaimedBytes > 0) return true;
  return providers.some(([, entry]) => providerUsefulBytes(entry) > 0);
}

function legacyParentChange(text: string): boolean {
  return /parent changed/i.test(text);
}

function rawErrorTokens(text: string): string[] {
  return text
    .split(/[\n,]/)
    .map((token) => token.trim())
    .filter(Boolean)
    .map((token) => token.replace(/^[a-z_][a-z0-9_]*:\s*/i, ""));
}

function hasIdentityEvidence(issues: unknown, texts: string[]): boolean {
  if (Array.isArray(issues) && issues.includes("cache_parent_changed")) return true;
  return texts.some((text) => legacyParentChange(text));
}

function categoryForCode(code: string): StorageHistoryIssueCategory | undefined {
  return Object.prototype.hasOwnProperty.call(ISSUE_CATEGORY_BY_CODE, code)
    ? ISSUE_CATEGORY_BY_CODE[code]
    : undefined;
}

function issueCategory(issue: unknown, identityChanged: boolean): StorageHistoryIssueCategory {
  if (typeof issue !== "string") return "unknown";
  if (identityChanged && (issue === "entry_read_failed" || issue === "entry_delete_failed")) {
    return "entries_changed";
  }
  return categoryForCode(issue) ?? "unknown";
}

function goCacheIssueCategories(
  issues: unknown,
  providerError: unknown,
  message: string | undefined,
): { categories: StorageHistoryIssueCategory[]; hasUnknownIssue: boolean } {
  const texts = [providerError, message].filter((text): text is string => typeof text === "string");
  const identityChanged = hasIdentityEvidence(issues, texts);
  let unrelatedCause = false;
  if (typeof providerError === "string") {
    for (const token of rawErrorTokens(providerError)) {
      if (legacyParentChange(token)) continue;
      if (categoryForCode(token) === undefined) unrelatedCause = true;
    }
  }
  const foldIdentityFailures = identityChanged && !unrelatedCause;
  const categories: StorageHistoryIssueCategory[] = [];
  const seen = new Set<StorageHistoryIssueCategory>();
  let hasUnknownIssue = false;
  const add = (category: StorageHistoryIssueCategory) => {
    if (category === "unknown") hasUnknownIssue = true;
    if (seen.has(category)) return;
    seen.add(category);
    categories.push(category);
  };
  if (issues !== undefined && issues !== null) {
    const list = Array.isArray(issues) ? issues : [undefined];
    for (const issue of list) add(issueCategory(issue, foldIdentityFailures));
  }
  if (identityChanged) add("entries_changed");
  if (unrelatedCause && !categories.some((category) => FAILURE_CATEGORIES.has(category))) {
    add("other");
  }
  return { categories, hasUnknownIssue };
}

/**
 * Reads Go-cache bytes and issue evidence from a maintenance result. The
 * returned outcome keeps stored payloads untouched and never turns retained
 * issue samples into exact counts.
 */
export function storageGoCacheResultSummary(
  result: Record<string, unknown>,
  message?: string,
): StorageGoCacheOutcome | null {
  const outer = recordValue(result.result) ?? result;
  const provider = recordValue(outer.go_cache);
  const cleanup = recordValue(provider?.result) ?? provider;
  if (!cleanup) return null;
  const reclaimedBytes = finiteNonNegative(cleanup.reclaimed_bytes);
  if (reclaimedBytes === null) return null;
  const { categories, hasUnknownIssue } = goCacheIssueCategories(
    cleanup.errors,
    provider?.error,
    message,
  );
  const skippedProviders = recordValue(outer.skipped_providers);
  return {
    reclaimedBytes,
    bytesAfter: finiteNonNegative(cleanup.bytes_after),
    skipped: cleanup.skipped === true,
    partial:
      cleanup.partial === true || (Array.isArray(cleanup.errors) && cleanup.errors.length > 0),
    issueCategories: categories,
    hasUnknownIssue,
    otherProvidersSkipped: Boolean(skippedProviders && Object.keys(skippedProviders).length > 0),
  };
}

function deriveStatus(
  run: StorageMaintenanceRun,
  providers: Array<[string, unknown]>,
  goCache: StorageGoCacheOutcome | null,
  hasProviderError: boolean,
): StorageRunDisplayStatus {
  if (!KNOWN_STATES.has(run.state)) return "unknown";
  if (PRESERVED_STATES.includes(run.state)) return run.state;
  if (run.state === "failed" && expectedGoCacheProgress(providers, run.message)) {
    return "incomplete";
  }
  if (hasProviderError) {
    return hasUsefulWork(providers, goCache) ? "partial" : "failed";
  }
  return run.state === "succeeded" ? "succeeded" : "failed";
}

/**
 * Derives one display outcome from every provider result. Stored run states and
 * payloads stay unchanged; unknown or malformed evidence stays a failure.
 */
export function storageRunSummary(run: StorageMaintenanceRun): StorageRunSummary {
  const providers = providerEntries(run.result);
  const hasProviderError = providers.some(([, entry]) => providerHasFailure(entry));
  const hasNonGoProviderError = providers.some(
    ([name, entry]) => name !== "go_cache" && providerHasFailure(entry),
  );
  const goCache = storageGoCacheResultSummary(run.result, run.message);
  const skippedProviders = recordValue(runResultRecord(run.result).skipped_providers);
  const settings = recordValue(run.settings_snapshot);
  const goCacheSettings = recordValue(settings?.go_cache);
  return {
    status: deriveStatus(run, providers, goCache, hasProviderError),
    goCache,
    hasProviderError,
    hasNonGoProviderError,
    otherProvidersSkipped: Boolean(skippedProviders && Object.keys(skippedProviders).length > 0),
    busyPolicyEnabled: goCacheSettings?.allow_cleanup_while_busy === true,
  };
}
