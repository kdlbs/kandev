import { renderHook } from "@testing-library/react";
import { beforeEach, expect, it, vi } from "vitest";
import { useSandboxSelectionModel } from "./use-sandbox-selection-model";

const reads = vi.hoisted(() => vi.fn());
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({ agentProfiles: { items: [] }, executors: { items: [] } }),
}));
vi.mock("./use-sandbox-binding", () => ({
  useSandboxBinding: () => ({
    saved: { workspace_id: "workspace", version: 3 },
    draft: { revision_id: "" },
    loading: false,
    saving: false,
  }),
}));
vi.mock("./use-sandbox-policy-reads", () => ({ useSandboxPolicyReads: reads }));

beforeEach(() => {
  reads.mockClear();
  reads.mockReturnValue({
    preview: { isSuccess: true, isFetching: false, data: { eligible: [] } },
  });
});

it("includes the task workflow step so the preview resolves inherited workflow policy", () => {
  renderHook(() =>
    useSandboxSelectionModel({
      scope: "task",
      targetId: "task",
      identity: "operator",
      canRead: true,
      canEdit: true,
      agentProfileId: "agent",
      executorId: "executor",
      workflowStepId: "workflow-step",
    }),
  );
  expect(reads.mock.calls.at(-1)?.[1]).toEqual({
    executor_id: "executor",
    workspace_id: "workspace",
    agent_profile_id: "agent",
    task_id: "task",
    workflow_step_id: "workflow-step",
  });
});

it("uses the selected workflow target for workflow settings", () => {
  renderHook(() =>
    useSandboxSelectionModel({
      scope: "workflow_step",
      targetId: "step",
      identity: "operator",
      canRead: true,
      canEdit: true,
      agentProfileId: "agent",
      executorId: "executor",
    }),
  );
  expect(reads.mock.calls.at(-1)?.[1]).toMatchObject({
    workflow_step_id: "step",
    task_id: undefined,
  });
});
