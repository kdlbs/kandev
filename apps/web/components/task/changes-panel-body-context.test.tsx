import type { ReactNode } from "react";
import type { StoreApi } from "zustand";
import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { ChangesPanelBodyProps } from "./changes-panel-data";
import type { ChangesHistoryTimelineRow } from "./changes-timeline-model";

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
const ARIA_EXPANDED = "aria-expanded";
const EXPANDED = "true";
const COLLAPSED = "false";

let appStore: StoreApi<AppState>;

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

function panelProps(): ChangesPanelBodyProps {
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
    commits: [commit("frontend-commit", "frontend"), commit("backend-commit", "backend")],
    pendingStageFiles: new Set(),
    reviewedCount: 0,
    totalFileCount: 0,
    aheadCount: 0,
    comparisonTargets: [],
    comparisonUnavailable: false,
    comparisonErrorCode: null,
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
  it("resets historical section, repository, and inline-directory expansion across task and environment changes", async () => {
    render(
      <TooltipProvider>
        <StateProvider>
          <ChangesPanelBody {...panelProps()} />
          <StoreCapture />
        </StateProvider>
      </TooltipProvider>,
    );
    setContext("task-a", "session-a", "environment-a");
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

    const frontendRepository = expandedRepository("frontend");
    fireEvent.click(frontendRepository);
    expect(frontendRepository.getAttribute(ARIA_EXPANDED)).toBe(COLLAPSED);
    fireEvent.click(frontendRepository);

    fireEvent.click(expandedCommit("frontend-commit"));
    const inlineDirectory = await screen.findByTestId(INLINE_DIRECTORY);
    fireEvent.click(inlineDirectory);
    expect(screen.queryByTestId(INLINE_FILE)).toBeNull();

    setContext("task-b", "session-b", "environment-b");
    const taskBSection = screen.getByTestId(COMMIT_SECTION_TOGGLE);
    expect(taskBSection.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    for (const repository of screen.getAllByTestId("commits-repo-header")) {
      expect(repository.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    }

    fireEvent.click(expandedCommit("frontend-commit"));
    const taskBDirectory = await screen.findByTestId(INLINE_DIRECTORY);
    expect(taskBDirectory.getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
    expect(screen.getByTestId(INLINE_FILE)).toBeTruthy();

    fireEvent.click(taskBSection);
    expect(taskBSection.getAttribute(ARIA_EXPANDED)).toBe(COLLAPSED);
    setContext("task-b", "session-b", "environment-c");
    expect(screen.getByTestId(COMMIT_SECTION_TOGGLE).getAttribute(ARIA_EXPANDED)).toBe(EXPANDED);
  });
});
