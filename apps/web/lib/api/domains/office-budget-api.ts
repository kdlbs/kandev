import { fetchJson, type ApiRequestOptions } from "../client";
import type { BudgetPolicy } from "@/lib/state/slices/office/types";

const BASE = "/api/v1/office";

type RawBudgetPolicy = Record<string, unknown>;

function budgetField(raw: RawBudgetPolicy, camelKey: string, snakeKey: string): unknown {
  return raw[camelKey] ?? raw[snakeKey];
}

function normalizeBudget(raw: unknown): BudgetPolicy {
  const budget = raw as RawBudgetPolicy;
  return {
    id: String(budgetField(budget, "id", "id") ?? ""),
    workspaceId: String(budgetField(budget, "workspaceId", "workspace_id") ?? ""),
    scopeType: budgetField(budget, "scopeType", "scope_type") as BudgetPolicy["scopeType"],
    scopeId: String(budgetField(budget, "scopeId", "scope_id") ?? ""),
    limitSubcents: Number(budgetField(budget, "limitSubcents", "limit_subcents") ?? 0),
    period: budgetField(budget, "period", "period") as BudgetPolicy["period"],
    alertThresholdPct: Number(budgetField(budget, "alertThresholdPct", "alert_threshold_pct") ?? 0),
    actionOnExceed: budgetField(
      budget,
      "actionOnExceed",
      "action_on_exceed",
    ) as BudgetPolicy["actionOnExceed"],
    createdAt: String(budgetField(budget, "createdAt", "created_at") ?? ""),
    updatedAt: String(budgetField(budget, "updatedAt", "updated_at") ?? ""),
  };
}

function budgetPayload(data: Partial<BudgetPolicy>): Record<string, unknown> {
  return {
    scope_type: data.scopeType,
    scope_id: data.scopeId,
    limit_subcents: data.limitSubcents,
    period: data.period,
    alert_threshold_pct: data.alertThresholdPct,
    action_on_exceed: data.actionOnExceed,
  };
}

type BudgetResponse = { budget: unknown };

export function listBudgets(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<{ budgets: unknown[] }>(
    `${BASE}/workspaces/${workspaceId}/budgets`,
    options,
  ).then((res) => ({ budgets: (res.budgets ?? []).map((budget) => normalizeBudget(budget)) }));
}

export function createBudget(
  workspaceId: string,
  data: Partial<BudgetPolicy>,
  options?: ApiRequestOptions,
) {
  return fetchJson<BudgetResponse>(`${BASE}/workspaces/${workspaceId}/budgets`, {
    ...options,
    init: { method: "POST", body: JSON.stringify(budgetPayload(data)), ...options?.init },
  }).then((res) => normalizeBudget(res.budget));
}

export function updateBudget(id: string, data: Partial<BudgetPolicy>, options?: ApiRequestOptions) {
  return fetchJson<BudgetResponse>(`${BASE}/budgets/${id}`, {
    ...options,
    init: { method: "PATCH", body: JSON.stringify(budgetPayload(data)), ...options?.init },
  }).then((res) => normalizeBudget(res.budget));
}

export function deleteBudget(id: string, options?: ApiRequestOptions) {
  return fetchJson<void>(`${BASE}/budgets/${id}`, {
    ...options,
    init: { method: "DELETE", ...options?.init },
  });
}
