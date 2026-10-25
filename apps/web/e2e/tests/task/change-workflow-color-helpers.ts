import { expect } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

export async function seedRemainingStepColors(apiClient: ApiClient, workflowId: string) {
  const cases = [
    ["Gray", "bg-slate-500", "var(--color-slate-500)"],
    ["Red", "bg-red-500", "var(--color-red-500)"],
    ["Orange", "bg-orange-500", "var(--color-orange-500)"],
    ["Yellow", "bg-yellow-500", "var(--color-yellow-500)"],
    ["Cyan", "bg-cyan-500", "var(--color-cyan-500)"],
    ["Indigo", "bg-indigo-500", "var(--color-indigo-500)"],
    ["Purple", "bg-purple-500", "var(--color-purple-500)"],
    ["Missing color", "", "var(--color-slate-500)"],
    ["Unsupported color", "not-a-color", "var(--color-slate-500)"],
    ["Custom hex", "#abcdef", "#abcdef"],
  ];
  const steps = [];
  for (const [name, color, cssColor] of cases) {
    const step = await apiClient.createWorkflowStep(workflowId, name, steps.length + 2);
    expect(
      (await apiClient.rawRequest("PUT", `/api/v1/workflow/steps/${step.id}`, { color })).ok,
    ).toBe(true);
    steps.push({ id: step.id, cssColor });
  }
  return steps;
}
