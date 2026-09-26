import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { PRMergeButton } from "./pr-merge-button";

const mergePRMock = vi.hoisted(() => vi.fn());
const MERGE_BUTTON_TEST_ID = "pr-merge-button";

vi.mock("@/lib/api/domains/github-api", () => ({ mergePR: mergePRMock }));
vi.mock("@/hooks/domains/github/use-github-status", () => ({
  useGitHubStatus: () => ({ status: null }),
}));
vi.mock("@/hooks/domains/github/use-repo-merge-methods", () => ({
  useRepoMergeMethods: () => null,
}));

const taskPR = {
  id: "pr-1",
  task_id: "task-1",
  owner: "acme",
  repo: "site",
  pr_number: 42,
  state: "open",
  review_state: "approved",
  checks_state: "success",
  mergeable_state: "blocked",
  review_count: 2,
  required_reviews: 2,
  pending_review_count: 0,
} as TaskPR;

function renderButton(onMerged = vi.fn()) {
  const initialState = { workspaces: { activeId: "workspace-1" } } as Partial<AppState>;
  render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <PRMergeButton taskPR={taskPR} onMerged={onMerged} />
      </ToastProvider>
    </StateProvider>,
  );
  return onMerged;
}

beforeEach(() => mergePRMock.mockReset());
afterEach(() => cleanup());

describe("PRMergeButton", () => {
  it.each([
    ["merged", "PR merged"],
    ["queued", "PR added to merge queue"],
  ] as const)("reports a %s terminal outcome", async (status, message) => {
    mergePRMock.mockResolvedValue({ status });
    const onMerged = renderButton();

    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TEST_ID));

    expect(await screen.findByText(message)).not.toBeNull();
    expect(onMerged).toHaveBeenCalledOnce();
    expect(screen.queryByTestId(MERGE_BUTTON_TEST_ID)).toBeNull();
  });

  it("remains retryable after a rejected merge", async () => {
    mergePRMock.mockRejectedValueOnce(new Error("merge rejected")).mockResolvedValue({
      status: "queued",
    });
    renderButton();

    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TEST_ID));
    expect(await screen.findByText("merge rejected")).not.toBeNull();
    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TEST_ID));

    await waitFor(() => expect(mergePRMock).toHaveBeenCalledTimes(2));
  });

  it("suppresses duplicate clicks while the request is pending", async () => {
    let resolve!: (value: { status: "queued" }) => void;
    mergePRMock.mockReturnValue(new Promise((done) => (resolve = done)));
    renderButton();
    const button = screen.getByTestId(MERGE_BUTTON_TEST_ID);

    fireEvent.click(button);
    fireEvent.click(button);
    expect(mergePRMock).toHaveBeenCalledOnce();

    resolve({ status: "queued" });
    expect(await screen.findByText("PR added to merge queue")).not.toBeNull();
  });

  it("does not apply a delayed acceptance to a different pull request", async () => {
    let resolve!: (value: { status: "queued" }) => void;
    mergePRMock.mockReturnValue(new Promise((done) => (resolve = done)));
    const initialState = { workspaces: { activeId: "workspace-1" } } as Partial<AppState>;
    const view = render(
      <StateProvider initialState={initialState}>
        <ToastProvider>
          <PRMergeButton taskPR={taskPR} />
        </ToastProvider>
      </StateProvider>,
    );

    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TEST_ID));
    const nextPR = { ...taskPR, id: "pr-2", pr_number: 43 };
    view.rerender(
      <StateProvider initialState={initialState}>
        <ToastProvider>
          <PRMergeButton taskPR={nextPR} />
        </ToastProvider>
      </StateProvider>,
    );
    resolve({ status: "queued" });

    await screen.findByText("PR added to merge queue");
    expect(screen.getByTestId(MERGE_BUTTON_TEST_ID)).not.toBeNull();
  });
});
