import { fetchJson } from "@/lib/api/client";
import type {
  ArchivedDataPolicy,
  ArchivedDataPolicyUpdate,
  ArchivedDataRetentionStatus,
} from "@/lib/types/archived-data-retention";

const BASE = "/api/v1/system/database/archived-data-retention";
export const fetchArchivedDataRetention = () =>
  fetchJson<ArchivedDataRetentionStatus>(BASE, { cache: "no-store" });
export const saveArchivedDataRetention = (policy: ArchivedDataPolicyUpdate) =>
  fetchJson<ArchivedDataRetentionStatus>(BASE, {
    init: { method: "PUT", body: JSON.stringify(policy) },
  });
// Analysis takes a draft policy, not a single age, because both targets are
// estimated in one read-only pass.
export const analyzeArchivedDataRetention = (policy: ArchivedDataPolicy) =>
  fetchJson<{ operation_id: string }>(`${BASE}/analyze`, {
    init: { method: "POST", body: JSON.stringify(policy) },
  });
export const runArchivedDataRetention = (revision: number) =>
  fetchJson<{ operation_id: string }>(`${BASE}/run`, {
    init: { method: "POST", body: JSON.stringify({ revision }) },
  });
export const cancelArchivedDataRetention = (operation_id: string) =>
  fetchJson<ArchivedDataRetentionStatus>(`${BASE}/cancel`, {
    init: { method: "POST", body: JSON.stringify({ operation_id }) },
  });
