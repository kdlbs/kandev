import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useDialogFormState } from "./task-create-dialog-state";

vi.mock("@/hooks/domains/github/use-branches-by-url", () => ({
  useBranchesByURL: () => ({ branches: () => [], loading: () => false, ensure() {} }),
}));
vi.mock("@/hooks/domains/github/use-pr-info-by-url", () => ({
  usePRInfoByURL: () => ({ info() {}, loading: () => false, ensure() {}, clear() {} }),
}));

describe("locked workflow context hydration", () => {
  it("preserves a typed title when the initial workspace arrives", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string | null }) =>
        useDialogFormState(true, workspaceId, "workflow-1", undefined, true),
      { initialProps: { workspaceId: null as string | null } },
    );
    act(() => result.current.setTaskName("Draft entered during loading"));
    rerender({ workspaceId: "workspace-a" });
    expect(result.current.taskName).toBe("Draft entered during loading");
  });

  it("preserves a draft when a provisional workspace resolves with its locked workflow", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId, workflowId }: { workspaceId: string; workflowId: string | null }) =>
        useDialogFormState(true, workspaceId, workflowId, undefined, true),
      { initialProps: { workspaceId: "active-workspace", workflowId: null as string | null } },
    );
    act(() => result.current.setTaskName("Draft entered during bootstrap"));
    rerender({ workspaceId: "dedicated-workspace", workflowId: "workflow-1" });
    expect(result.current.taskName).toBe("Draft entered during bootstrap");
  });

  it("resets a draft when an established workspace changes", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) =>
        useDialogFormState(true, workspaceId, "workflow-1", undefined, true),
      { initialProps: { workspaceId: "workspace-a" } },
    );
    act(() => result.current.setTaskName("Workspace A draft"));
    rerender({ workspaceId: "workspace-b" });
    expect(result.current.taskName).toBe("");
  });

  it("resets a draft when the workspace changes while its workflow is still unresolved", () => {
    const { result, rerender } = renderHook(
      ({ workspaceId }: { workspaceId: string }) =>
        useDialogFormState(true, workspaceId, null, undefined, true),
      { initialProps: { workspaceId: "workspace-a" } },
    );
    act(() => result.current.setTaskName("Workspace A draft"));
    rerender({ workspaceId: "workspace-b" });
    expect(result.current.taskName).toBe("");
  });
});
