import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import type { AppState } from "@/lib/state/store";
import type { TaskPR } from "@/lib/types/github";
import { PRMergeButton } from "./pr-merge-button";
import { TaskWorkspaceProvider } from "@/components/task/task-workspace-context";

const mergePRMock = vi.hoisted(() => vi.fn());
const accessMock = vi.hoisted(() => vi.fn());
const refreshAccessMock = vi.hoisted(() => vi.fn());
const methodsMock = vi.hoisted(() => vi.fn());
const ENTERPRISE_HOST = "git.company.example";
const MERGE_BUTTON_TEST_ID = "pr-merge-button";

vi.mock("@/lib/api/domains/github-api", () => ({ mergePR: mergePRMock }));
vi.mock("@/hooks/domains/github/use-repo-merge-access", () => ({
  useRepoMergeAccess: accessMock,
}));
vi.mock("@/hooks/domains/github/use-repo-merge-methods", () => ({
  useRepoMergeMethods: methodsMock,
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

function renderButton(onMerged = vi.fn(), pr = taskPR, compact = false) {
  const initialState = { workspaces: { activeId: "workspace-1" } } as Partial<AppState>;
  render(
    <StateProvider initialState={initialState}>
      <ToastProvider>
        <PRMergeButton taskPR={pr} onMerged={onMerged} compact={compact} />
      </ToastProvider>
    </StateProvider>,
  );
  return onMerged;
}

beforeEach(() => {
  mergePRMock.mockReset();
  refreshAccessMock.mockReset();
  methodsMock.mockReset().mockReturnValue(null);
  accessMock.mockReset().mockReturnValue({
    state: "allowed",
    actor: { kind: "human", login: "writer" },
    refresh: refreshAccessMock,
  });
});
afterEach(() => cleanup());

describe("PRMergeButton permissions", () => {
  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.17
  it("does not expose a merge before access is confirmed", () => {
    accessMock.mockReturnValue({ state: "loading", actor: null, refresh: refreshAccessMock });
    renderButton();
    expect(screen.queryByTestId(MERGE_BUTTON_TEST_ID)).toBeNull();
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.16
  it.each([false, true])("shows denied access without merge methods (compact=%s)", (compact) => {
    accessMock.mockReturnValue({
      state: "denied",
      actor: { kind: "human", login: "reader" },
      refresh: refreshAccessMock,
    });
    methodsMock.mockReturnValue({ merge: true, squash: true, rebase: true });
    renderButton(undefined, { ...taskPR, mergeable_state: "clean" }, compact);
    expect(screen.getByRole("status").textContent).toBe("reader cannot merge into acme/site.");
    expect(screen.queryByTestId(MERGE_BUTTON_TEST_ID)).toBeNull();
    expect(screen.queryByTestId("pr-merge-button-more")).toBeNull();
    expect(screen.queryByTestId("pr-merge-method-squash")).toBeNull();
    expect(mergePRMock).not.toHaveBeenCalled();
  });

  // @covers AC-INTEGRATIONS-GITHUB-PR-MERGE-QUEUE-001.20
  it("keeps a permitted actor blocked when checks fail", () => {
    renderButton(undefined, { ...taskPR, checks_state: "failure" });
    expect(screen.queryByTestId(MERGE_BUTTON_TEST_ID)).toBeNull();
    expect(mergePRMock).not.toHaveBeenCalled();
  });

  it("attributes the merge to the confirmed App actor", () => {
    accessMock.mockReturnValue({
      state: "allowed",
      actor: { kind: "app", login: "release-bot" },
      refresh: refreshAccessMock,
    });
    renderButton();
    expect(screen.getByText("as release-bot GitHub App")).not.toBeNull();
    expect(screen.getByTestId(MERGE_BUTTON_TEST_ID)).not.toBeNull();
  });

  it("rechecks unknown permissions without submitting a merge", () => {
    accessMock.mockReturnValue({ state: "unknown", actor: null, refresh: refreshAccessMock });
    renderButton();
    fireEvent.click(screen.getByRole("button", { name: "Check again" }));
    expect(refreshAccessMock).toHaveBeenCalledOnce();
    expect(mergePRMock).not.toHaveBeenCalled();
  });
});

describe("PRMergeButton", () => {
  it("merges an embedded original task using its workspace rather than active navigation", async () => {
    mergePRMock.mockResolvedValue({ status: "merged" });
    render(
      <StateProvider initialState={{ workspaces: { activeId: "alpha", items: [] } }}>
        <ToastProvider>
          <TaskWorkspaceProvider
            taskId={taskPR.task_id}
            workspaceId="beta"
            sessionId="beta-session"
          >
            <PRMergeButton taskPR={{ ...taskPR, github_host: ENTERPRISE_HOST }} />
          </TaskWorkspaceProvider>
        </ToastProvider>
      </StateProvider>,
    );
    expect(accessMock).toHaveBeenCalledWith("beta", "acme", "site", true, ENTERPRISE_HOST);
    expect(methodsMock).toHaveBeenCalledWith("beta", "acme", "site", ENTERPRISE_HOST);
    fireEvent.click(screen.getByTestId(MERGE_BUTTON_TEST_ID));
    await waitFor(() =>
      expect(mergePRMock).toHaveBeenCalledWith("beta", "acme", "site", 42, {
        mergeMethod: undefined,
        expectedHost: ENTERPRISE_HOST,
      }),
    );
  });

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
    expect(refreshAccessMock).toHaveBeenCalledOnce();
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
