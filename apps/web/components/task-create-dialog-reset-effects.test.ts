import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useDialogFormState } from "./task-create-dialog-state";

vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({ branches: () => [], loading: () => false, ensure() {} }),
}));
vi.mock("@/hooks/domains/github/use-pr-info-by-url", () => ({
  usePRInfoByURL: () => ({ info() {}, loading: () => false, ensure() {}, clear() {} }),
}));

const WORKSPACE_A = "workspace-a";
const WORKFLOW_ID = "workflow-1";

describe("locked workflow context hydration", () => {
  it("preserves a typed title when the initial workspace arrives", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) =>
        useDialogFormState(true, workspaceId, WORKFLOW_ID, undefined, true),
      { initialProps: { workspaceId: null as string | null } },
    );
    act(() => result.current.setTaskName("Draft entered during loading"));
    rerender({ workspaceId: WORKSPACE_A });
    expect(result.current.taskName).toBe("Draft entered during loading");
  });

  it("preserves a draft when a provisional workspace resolves with its locked workflow", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId, workflowId }: { workspaceId: string; workflowId: string | null }) =>
        useDialogFormState(true, workspaceId, workflowId, undefined, true),
      { initialProps: { workspaceId: "active-workspace", workflowId: null as string | null } },
    );
    act(() => result.current.setTaskName("Draft entered during bootstrap"));
    rerender({ workspaceId: "dedicated-workspace", workflowId: WORKFLOW_ID });
    expect(result.current.taskName).toBe("Draft entered during bootstrap");
  });

  it("resets a draft when an established workspace changes", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) =>
        useDialogFormState(true, workspaceId, WORKFLOW_ID, undefined, true),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    act(() => result.current.setTaskName("Workspace A draft"));
    rerender({ workspaceId: "workspace-b" });
    expect(result.current.taskName).toBe("");
  });

  it("resets a draft when the workspace changes while its workflow is still unresolved", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) =>
        useDialogFormState(true, workspaceId, null, undefined, true),
      { initialProps: { workspaceId: WORKSPACE_A } },
    );
    act(() => result.current.setTaskName("Workspace A draft"));
    rerender({ workspaceId: "workspace-b" });
    expect(result.current.taskName).toBe("");
  });
});

describe("built-in task creation consent lifecycle", () => {
  const draft = {
    workspaceId: WORKSPACE_A,
    preferences: {
      version: 1 as const,
      expected_workspace_policy_revision: 0,
      packages: { configuration: "on" as const },
    },
  };

  it("clears consent on a new task open cycle", () => {
    const { result, rerender } = renderHook(
      ({ open }) => useDialogFormState(open, WORKSPACE_A, null),
      { initialProps: { open: true } },
    );
    act(() => result.current.setBuiltinMCPPreferences(draft));
    rerender({ open: false });
    rerender({ open: true });
    expect(result.current.builtinMCPPreferences).toBeUndefined();
  });

  it("clears consent when a locked workflow resolves to another workspace", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId, workflowId }: { workspaceId: string; workflowId: string | null }) =>
        useDialogFormState(true, workspaceId, workflowId, undefined, true),
      { initialProps: { workspaceId: WORKSPACE_A, workflowId: null as string | null } },
    );
    act(() => result.current.setBuiltinMCPPreferences(draft));
    rerender({ workspaceId: "workspace-b", workflowId: WORKFLOW_ID });
    expect(result.current.builtinMCPPreferences).toBeUndefined();
  });
});
