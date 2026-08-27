import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import type { CreateTaskResponse } from "@/lib/types/http";
import type { TaskCreateSubmit } from "./task-create-dialog-types";

const defaultCreateTaskMock = vi.fn();
const WORKSPACE_ID = "workspace-1";
const WORKFLOW_ID = "workflow-1";

vi.mock("@/lib/api", () => ({
  createTask: (...args: unknown[]) => defaultCreateTaskMock(...args),
}));

import { useFreshBranchConsent } from "./task-create-dialog-fresh-branch-consent";

const response = { id: "task-1" } as CreateTaskResponse;

function renderConsent(createTask?: TaskCreateSubmit) {
  return renderHook(() =>
    useFreshBranchConsent({
      isFreshBranchActive: true,
      workspaceId: WORKSPACE_ID,
      repositoryLocalPath: "/repo",
      toast: vi.fn(),
      createTask,
    }),
  );
}

beforeEach(() => {
  defaultCreateTaskMock.mockReset();
  defaultCreateTaskMock.mockResolvedValue(response);
});

describe("useFreshBranchConsent task creation", () => {
  it("keeps the REST task creator as the default", async () => {
    const { result } = renderConsent();

    await expect(
      result.current.createTaskWithFreshBranchRetry(
        (consented) => ({
          workspace_id: WORKSPACE_ID,
          workflow_id: WORKFLOW_ID,
          title: consented.join(","),
        }),
        ["first"],
      ),
    ).resolves.toBe(response);

    expect(defaultCreateTaskMock).toHaveBeenCalledWith({
      workspace_id: WORKSPACE_ID,
      workflow_id: WORKFLOW_ID,
      title: "first",
    });
  });

  it("uses the injected task creator for the first attempt and fresh-branch retry", async () => {
    const createTask = vi
      .fn()
      .mockRejectedValueOnce(new ApiError("working tree changed", 409, { dirty_files: ["new.ts"] }))
      .mockResolvedValueOnce(response);
    const { result } = renderConsent(createTask);

    let submission!: Promise<CreateTaskResponse | null>;
    act(() => {
      submission = result.current.createTaskWithFreshBranchRetry(
        (consented) => ({
          workspace_id: WORKSPACE_ID,
          workflow_id: WORKFLOW_ID,
          title: consented.join(","),
        }),
        ["old.ts"],
      );
    });

    await waitFor(() => expect(result.current.pendingDiscard?.dirtyFiles).toEqual(["new.ts"]));
    act(() => result.current.pendingDiscard?.resolve(true));

    await expect(submission).resolves.toBe(response);
    expect(createTask).toHaveBeenNthCalledWith(1, {
      workspace_id: WORKSPACE_ID,
      workflow_id: WORKFLOW_ID,
      title: "old.ts",
    });
    expect(createTask).toHaveBeenNthCalledWith(2, {
      workspace_id: WORKSPACE_ID,
      workflow_id: WORKFLOW_ID,
      title: "new.ts",
    });
    expect(defaultCreateTaskMock).not.toHaveBeenCalled();
  });
});
