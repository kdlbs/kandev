import { fetchJson } from "@/lib/api/client";
import type {
  ToolPayloadAge,
  ToolPayloadPolicyUpdate,
  ToolPayloadRetentionStatus,
} from "@/lib/types/tool-payload-retention";

const BASE = "/api/v1/system/database/tool-payload-retention";
export const fetchToolPayloadRetention = () =>
  fetchJson<ToolPayloadRetentionStatus>(BASE, { cache: "no-store" });
export const saveToolPayloadRetention = (policy: ToolPayloadPolicyUpdate) =>
  fetchJson<ToolPayloadRetentionStatus>(BASE, {
    init: { method: "PUT", body: JSON.stringify(policy) },
  });
export const analyzeToolPayloadRetention = (age: ToolPayloadAge) =>
  fetchJson<{ operation_id: string }>(`${BASE}/analyze`, {
    init: { method: "POST", body: JSON.stringify({ age }) },
  });
export const runToolPayloadRetention = (revision: number) =>
  fetchJson<{ operation_id: string }>(`${BASE}/run`, {
    init: { method: "POST", body: JSON.stringify({ revision }) },
  });
export const cancelToolPayloadRetention = (operation_id: string) =>
  fetchJson<ToolPayloadRetentionStatus>(`${BASE}/cancel`, {
    init: { method: "POST", body: JSON.stringify({ operation_id }) },
  });
