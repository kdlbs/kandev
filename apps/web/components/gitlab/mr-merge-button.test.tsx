import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { ToastProvider } from "@/components/toast-provider";
import type { TaskMR } from "@/lib/types/gitlab";

const mergeMR = vi.hoisted(() => vi.fn());

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { workspaces: { activeId: string } }) => unknown) =>
    selector({ workspaces: { activeId: "ws-1" } }),
}));

vi.mock("@/lib/api/domains/gitlab-api", () => ({ mergeMR }));

import { MRMergeButton } from "./mr-merge-button";

const MERGE_BUTTON_TESTID = "mr-merge-button";

function makeMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return {
    id: "id",
    task_id: "task-1",
    host: "https://gitlab.com",
    project_path: "group/project",
    mr_iid: 7,
    mr_url: "",
    mr_title: "Test MR",
    head_branch: "feature",
    base_branch: "main",
    author_username: "alice",
    state: "open",
    approval_state: "",
    pipeline_state: "success",
    merge_status: "can_be_merged",
    draft: false,
    approval_count: 0,
    required_approvals: 0,
    pipeline_jobs_total: 0,
    pipeline_jobs_pass: 0,
    reviewer_count: 0,
    unapproved_reviewers: 0,
    unresolved_discussions: 0,
    created_at: "",
    updated_at: "",
    ...overrides,
  } as TaskMR;
}

function readyMR(overrides: Partial<TaskMR> = {}): TaskMR {
  return makeMR({
    state: "open",
    draft: false,
    pipeline_state: "success",
    unresolved_discussions: 0,
    detailed_merge_status: "mergeable",
    ...overrides,
  });
}

function renderButton(mr: TaskMR) {
  return render(
    <ToastProvider>
      <MRMergeButton mr={mr} />
    </ToastProvider>,
  );
}

beforeEach(() => {
  mergeMR.mockReset();
});

afterEach(() => cleanup());

describe("MRMergeButton", () => {
  it("renders nothing when the MR is not ready to merge", () => {
    renderButton(makeMR({ pipeline_state: "failed" }));
    expect(screen.queryByTestId(MERGE_BUTTON_TESTID)).toBeNull();
  });

  it("renders the Merge action when the MR is ready", () => {
    renderButton(readyMR());
    expect(screen.getByTestId(MERGE_BUTTON_TESTID).textContent).toContain("Merge");
  });

  it("calls mergeMR with the MR's identity on click", async () => {
    mergeMR.mockResolvedValue({});
    const mr = readyMR({ project_path: "group/project", mr_iid: 42, host: "https://gitlab.com" });
    renderButton(mr);
    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TESTID));
    await vi.waitFor(() => expect(mergeMR).toHaveBeenCalledTimes(1));
    expect(mergeMR).toHaveBeenCalledWith({
      workspaceId: "ws-1",
      project: "group/project",
      iid: 42,
      host: "https://gitlab.com",
      squash: false,
    });
  });

  it("disables the button and shows an in-flight label while merging", async () => {
    let resolveMerge: (() => void) | undefined;
    mergeMR.mockReturnValue(
      new Promise<void>((resolve) => {
        resolveMerge = resolve;
      }),
    );
    renderButton(readyMR());
    const button = screen.getByTestId(MERGE_BUTTON_TESTID);
    fireEvent.click(button);
    await vi.waitFor(() => expect(button.textContent).toContain("Merging"));
    expect((button as HTMLButtonElement).disabled).toBe(true);
    resolveMerge?.();
  });
});
