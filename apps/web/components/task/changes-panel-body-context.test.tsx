import type { ReactNode } from "react";
import type { StoreApi } from "zustand";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { ChangesPanelBodyProps } from "./changes-panel-data";
import type { ChangesHistoryTimelineRow } from "./changes-timeline-model";
import {
  useChangesInlineCommitDetails,
  useChangesPanelContextIdentity,
} from "./use-changes-inline-commit-details";

const mocks = vi.hoisted(() => ({
  requestCommitDetail: vi.fn(),
  toast: vi.fn(),
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

vi.mock("./commit-detail-request", () => ({
  requestCommitDetail: mocks.requestCommitDetail,
  CommitDetailProtocolError: class CommitDetailProtocolError extends Error {
    constructor() {
      super("Invalid commit detail response");
    }
  },
}));

vi.mock("./changes-panel-dialogs", () => ({
  DiscardDialog: () => null,
  AmendDialog: () => null,
  ResetDialog: () => null,
}));

vi.mock("./changes-panel-timeline", () => ({
  ReviewProgressBar: () => null,
}));

vi.mock("./changes-timeline-working-tree", () => ({
  ChangesWorkingTree: ({
    historyRowsBefore,
    historyRowsAfter,
    renderHistoryRow,
  }: {
    historyRowsBefore?: ChangesHistoryTimelineRow[];
    historyRowsAfter?: ChangesHistoryTimelineRow[];
    renderHistoryRow?: (row: ChangesHistoryTimelineRow) => ReactNode;
  }) => (
    <div data-testid="history-owner">
      {[...(historyRowsBefore ?? []), ...(historyRowsAfter ?? [])].map((row) =>
        renderHistoryRow?.(row),
      )}
    </div>
  ),
}));

import { ChangesPanelBody } from "./changes-panel-body";

const COMMIT_SECTION_TOGGLE = "commits-section-collapse-toggle";
const INLINE_DIRECTORY = "commit-file-tree-dir-src";
const INLINE_FILE = "commit-file-src-file.ts";
const FRONTEND_REPOSITORY = "frontend";
const BACKEND_REPOSITORY = "backend";
const FRONTEND_COMMIT = "frontend-commit";
const TASK_A_CONTEXT = ["task-a", "session-a", "environment-a"] as const;
const TASK_B_CONTEXT = ["task-b", "session-b", "environment-b"] as const;
const TASK_B_NEW_ENVIRONMENT_CONTEXT = ["task-b", "session-b", "environment-c"] as const;
const ARIA_EXPANDED = "aria-expanded";
const EXPANDED = "true";
const COLLAPSED = "false";

let appStore: StoreApi<AppState>;

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

function StoreCapture() {
  appStore = useAppStoreApi();
  return null;
}

function setContext(taskId: string, sessionId: string, environmentId: string) {
  act(() => {
    appStore.setState((state) => ({
      ...state,
      tasks: { ...state.tasks, activeTaskId: taskId, activeSessionId: sessionId },
      environmentIdBySessionId: {
        ...state.environmentIdBySessionId,
        [sessionId]: environmentId,
      },
    }));
  });
}

type BodyTestProps = Omit<
  ChangesPanelBodyProps,
  "inlineCommitDetails" | "inlineCommitDetailVersion" | "contextKey"
>;

function panelProps(): BodyTestProps {
  const noop = () => {};
  const commit = (sha: string, repository_name: string) => ({
    commit_sha: sha,
    commit_message: `Commit ${sha}`,
    insertions: 1,
    deletions: 0,
    repository_name,
  });
  return {
    hasAnything: true,
    hasUnstaged: false,
    hasStaged: false,
    hasCommits: true,
    hasPRFiles: false,
    hasPRCommits: false,
    relation: { presentation: "combined" } as unknown as ChangesPanelBodyProps["relation"],
    resolution: {} as ChangesPanelBodyProps["resolution"],
    resolutionTarget: null,
    providerPRNumber: undefined,
    pushDisabled: false,
    pullDisabled: false,
    canPush: false,
    canCreatePR: false,
    existingPrUrl: undefined,
    unstagedFiles: [],
    stagedFiles: [],
    prFiles: [],
    prCommits: [],
    commits: [
      commit(FRONTEND_COMMIT, FRONTEND_REPOSITORY),
      commit("backend-commit", BACKEND_REPOSITORY),
    ],
    pendingStageFiles: new Set(),
    reviewedCount: 0,
    totalFileCount: 0,
    aheadCount: 0,
    comparisonTargets: [],
    comparisonUnavailable: false,
    comparisonErrorCode: null,
    gitStatus: {
      hasPriorData: false,
      membershipReady: false,
      refreshPending: false,
      loading: false,
      unavailable: false,
      detailsPending: false,
      failedRepositories: [],
      showEmpty: false,
    },
    isLoading: false,
    loadingOperation: null,
    dialogs: {} as ChangesPanelBodyProps["dialogs"],
    onOpenDiffFile: noop,
    onEditFile: noop,
    onOpenCommitDetail: noop,
    onRevertCommit: noop,
    onStageAll: noop,
    onUnstageAll: noop,
    onStage: async () => {},
    onUnstage: async () => {},
    onBulkStage: noop,
    onBulkUnstage: noop,
    onBulkDiscard: noop,
    onPush: noop,
    onForcePush: noop,
    stagedFileCount: 0,
    stagedAdditions: 0,
    stagedDeletions: 0,
    repoDisplayName: (repositoryName) => repositoryName,
  };
}

function OwnedChangesPanelBody(props: BodyTestProps) {
  const context = useChangesPanelContextIdentity();
  const details = useChangesInlineCommitDetails(context);
  return (
    <ChangesPanelBody
      {...props}
      inlineCommitDetails={details.state}
      inlineCommitDetailVersion={details.version}
      contextKey={context.contextKey}
    />
  );
}

function expandedRepository(name: string): HTMLElement {
  const row = screen
    .getAllByTestId("commits-repo-header")
    .find((element) => element.textContent?.includes(name));
  if (!row) throw new Error(`Missing ${name} history repository`);
  return row;
}

function expandedCommit(sha: string): HTMLElement {
  return screen
    .getByTestId(`commit-row-${sha.slice(0, 7)}`)
    .querySelector("[data-testid='commit-toggle']")!;
}

beforeEach(() => {
  mocks.toast.mockReset();
  mocks.requestCommitDetail.mockReset();
  mocks.requestCommitDetail.mockResolvedValue({
    source: "local",
    success: true,
    files: {
      "src/file.ts": {
        path: "src/file.ts",
        status: "modified",
        staged: false,
        additions: 1,
        deletions: 0,
      },
    },
  });
});

afterEach(cleanup);

describe("ChangesPanelBody history context ownership", () => {
  it("keeps a failed local commit detail request retryable", async () => {
    mocks.requestCommitDetail
      .mockReset()
      .mockResolvedValueOnce({ source: "local", success: false })
      .mockResolvedValueOnce({
        source: "local",
        success: true,
        files: {
          "src/file.ts": {
            path: "src/file.ts",
            status: "modified",
            staged: false,
            additions: 1,
            deletions: 0,
          },
        },
      });
    render(
      <TooltipProvider>
        <StateProvider>
          <OwnedChangesPanelBody {...panelProps()} />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );
    setContext(...TASK_A_CONTEXT);
    act(() => {
      appStore.setState((state) => ({
        ...state,
        userSettings: { ...state.userSettings, changesPanelLayout: "tree" },
      }));
    });

    fireEvent.click(expandedCommit(FRONTEND_COMMIT));
    const failure = await screen.findByRole("alert");
    expect(failure.textContent).not.toContain("No files in this commit");
    expect(mocks.requestCommitDetail).toHaveBeenCalledTimes(1);

    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(await screen.findByTestId(INLINE_FILE)).toBeTruthy();
    expect(mocks.requestCommitDetail).toHaveBeenCalledTimes(2);
  });

  it("resets historical section, repository, and inline-directory expansion across task and environment changes", async () => {
    render(
      <TooltipProvider>
        <StateProvider>
          <OwnedChangesPanelBody {...panelProps()} />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );
    setContext(...TASK_A_CONTEXT);
    act(() => {
      appStore.setState((state) => ({
        ...state,
        userSettings: { ...state.userSettings, changesPanelLayout: "tree" },
      }));
    });

    const sectionToggle = screen.getByTestId(COMMIT_SECTION_TOGGLE);
    expect(sectionToggle.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    fireEvent.click(sectionToggle);
    expect(sectionToggle.getAttribute(ARIA_EXPANDED)).toBe(COLLAPSED);
    fireEvent.click(sectionToggle);

    const frontendRepository = expandedRepository(FRONTEND_REPOSITORY);
    fireEvent.click(frontendRepository);
    expect(frontendRepository.getAttribute(ARIA_EXPANDED)).toBe(COLLAPSED);
    fireEvent.click(frontendRepository);

    fireEvent.click(expandedCommit(FRONTEND_COMMIT));
    const inlineDirectory = await screen.findByTestId(INLINE_DIRECTORY);
    fireEvent.click(inlineDirectory);
    expect(screen.queryByTestId(INLINE_FILE)).toBeNull();

    setContext(...TASK_B_CONTEXT);
    const taskBSection = screen.getByTestId(COMMIT_SECTION_TOGGLE);
    expect(taskBSection.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    for (const repository of screen.getAllByTestId("commits-repo-header")) {
      expect(repository.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    }

    fireEvent.click(expandedCommit(FRONTEND_COMMIT));
    const taskBDirectory = await screen.findByTestId(INLINE_DIRECTORY);
    expect(taskBDirectory.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    expect(screen.getByTestId(INLINE_FILE)).toBeTruthy();

    fireEvent.click(taskBSection);
    expect(taskBSection.getAttribute(ARIA_EXPANDED)).toBe(COLLAPSED);
    setContext(...TASK_B_NEW_ENVIRONMENT_CONTEXT);
    expect(screen.getByTestId(COMMIT_SECTION_TOGGLE).getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
  });
});

describe("ChangesPanelBody Git status presentation", () => {
  it("keeps refresh failure feedback out of the Changes body", () => {
    render(
      <TooltipProvider>
        <StateProvider>
          <OwnedChangesPanelBody
            {...panelProps()}
            gitStatus={{ ...panelProps().gitStatus, unavailable: true }}
          />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );

    expect(screen.queryByTestId("git-status-notice")).toBeNull();
    expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
    expect(screen.queryByText("Your changed files will appear here")).toBeNull();
  });

  it("shows the clean message only after complete membership is ready", () => {
    const renderPanel = (gitStatusMembershipReady: boolean) =>
      render(
        <TooltipProvider>
          <StateProvider>
            <OwnedChangesPanelBody
              {...panelProps()}
              hasAnything={false}
              hasCommits={false}
              gitStatus={{
                ...panelProps().gitStatus,
                membershipReady: gitStatusMembershipReady,
                showEmpty: gitStatusMembershipReady,
              }}
            />
            <StoreCapture />
          </StateProvider>
        </TooltipProvider>,
      );

    const view = renderPanel(false);
    expect(screen.queryByText("Your changed files will appear here")).toBeNull();
    view.unmount();

    renderPanel(true);
    expect(screen.getByText("Your changed files will appear here")).toBeTruthy();
  });
});

describe("ChangesPanelBody inline request retirement", () => {
  it("does not normalize inline files returned after the panel changes context", async () => {
    const request = deferred<unknown>();
    mocks.requestCommitDetail.mockReturnValueOnce(request.promise);
    const staleFilesRead = vi.fn();
    const staleFiles = new Proxy(
      {
        "src/stale.ts": {
          path: "src/stale.ts",
          status: "modified",
          staged: false,
          additions: 1,
          deletions: 0,
        },
      },
      {
        ownKeys(target) {
          staleFilesRead();
          return Reflect.ownKeys(target);
        },
      },
    );
    render(
      <TooltipProvider>
        <StateProvider>
          <OwnedChangesPanelBody {...panelProps()} />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );
    setContext(...TASK_A_CONTEXT);
    fireEvent.click(expandedCommit(FRONTEND_COMMIT));
    expect(mocks.requestCommitDetail).toHaveBeenCalledOnce();

    setContext(...TASK_B_CONTEXT);
    await act(async () => {
      request.resolve({ source: "local", success: true, files: staleFiles });
      await request.promise;
      await Promise.resolve();
    });

    expect(staleFilesRead).not.toHaveBeenCalled();
    expect(screen.queryByTestId("commit-file-stale-file.ts")).toBeNull();
  });

  it("does not show an error toast for an inline request rejected after unmount", async () => {
    const request = deferred<unknown>();
    mocks.requestCommitDetail.mockReturnValueOnce(request.promise);
    const view = render(
      <TooltipProvider>
        <StateProvider>
          <OwnedChangesPanelBody {...panelProps()} />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );
    setContext(...TASK_A_CONTEXT);
    fireEvent.click(expandedCommit(FRONTEND_COMMIT));
    expect(mocks.requestCommitDetail).toHaveBeenCalledOnce();

    view.unmount();
    await act(async () => {
      request.reject(new Error("stale request failed"));
      await request.promise.catch(() => undefined);
      await Promise.resolve();
    });

    expect(mocks.toast).not.toHaveBeenCalled();
  });
});
