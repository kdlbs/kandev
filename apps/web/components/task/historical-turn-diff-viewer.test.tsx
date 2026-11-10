import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { HistoricalTurnDiffViewer } from "./historical-turn-diff-viewer";
import { ApiError } from "@/lib/api/client";

const mock = vi.hoisted(() => ({
  summary: {
    id: "set-1",
    task_id: "task-1",
    session_id: "session-1",
    turn_id: "turn-1",
    revision: 1,
    availability: "ready" as const,
    complete: false,
    summary_complete: true,
    content_complete: false,
    turn_ordinal: 2,
    fallback_anchor: "turn-changes:turn-1",
    file_count: 120,
    binary_file_count: 0,
    unknown_count_file_count: 0,
    repository_count: 1,
    repositories: [
      {
        id: "repo-1",
        checkout_id: "checkout-1",
        availability: "ready" as const,
        enumeration_complete: true,
        comparison_complete: true,
        content_complete: false,
        export_state: "queued" as "queued" | "running" | "retrying" | "complete" | "unavailable",
      },
    ],
  },
  content: { patch: null as string | null, loading: false, error: null as unknown },
}));

vi.mock("@/hooks/domains/session/use-turn-changes", () => ({
  useSessionTurnChanges: () => ({
    summaries: [mock.summary],
    loading: false,
    error: null,
    reload: vi.fn(),
    loadChangeSet: vi.fn(),
    hasMore: false,
    loadMore: vi.fn(),
  }),
}));

vi.mock("@/hooks/domains/session/use-turn-change-files", () => ({
  useTurnChangeFiles: () => ({
    filesByRepository: {},
    fileTotalsByRepository: {},
    loading: false,
    error: null,
    hasMore: false,
    loadMore: vi.fn(),
    retry: vi.fn(),
  }),
  useTurnChangeContent: () => mock.content,
}));

vi.mock("@/components/diff/file-diff-viewer", () => ({
  FileDiffViewer: ({ diff }: { diff: string }) => <div data-testid="file-diff">{diff}</div>,
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => key,
  }),
}));

afterEach(cleanup);

const LOADING_COPY = "task:turnChangesLoading";
const LATE_PATCH = "late file diff";

const target = {
  sessionId: "session-1",
  changeSetId: "set-1",
  repositoryChangeId: "repo-1",
  checkoutId: "checkout-1",
  fileChangeId: "late-file",
  path: "docs/late-file.md",
};

function resetMock() {
  mock.summary.repositories[0].export_state = "queued";
  mock.summary.repositories[0].content_complete = false;
  mock.content = {
    patch: null,
    loading: false,
    error: new ApiError("not ready", 404, {}) as unknown,
  };
}

beforeEach(resetMock);

it("keeps a synthetic later-page selection loading while its repository is pending", () => {
  render(<HistoricalTurnDiffViewer target={{ ...target }} />);
  expect(screen.getByText(LOADING_COPY)).toBeTruthy();
});

it("shows a terminal content error instead of masking it as pending", () => {
  mock.summary.repositories[0].export_state = "unavailable";
  render(<HistoricalTurnDiffViewer target={{ ...target }} />);
  expect(screen.getByText("task:turnChangesContentUnavailable")).toBeTruthy();
  expect(screen.queryByText(LOADING_COPY)).toBeNull();
});

it("shows a loaded patch for a synthetic later-page selection", () => {
  mock.content = { patch: LATE_PATCH, loading: false, error: null };
  render(<HistoricalTurnDiffViewer target={{ ...target }} />);
  expect(screen.getByTestId("file-diff").textContent).toBe(LATE_PATCH);
  expect(screen.queryByText(LOADING_COPY)).toBeNull();
});

it("retries a synthetic later-page selection when its repository settles", () => {
  const { rerender } = render(<HistoricalTurnDiffViewer target={{ ...target }} />);
  expect(screen.getByText(LOADING_COPY)).toBeTruthy();

  mock.summary.repositories[0].export_state = "complete";
  mock.content = { patch: LATE_PATCH, loading: false, error: null };
  rerender(<HistoricalTurnDiffViewer target={{ ...target }} />);

  expect(screen.getByTestId("file-diff").textContent).toBe(LATE_PATCH);
  expect(screen.queryByText(LOADING_COPY)).toBeNull();
});

it("surfaces a terminal content error for a synthetic later-page selection", () => {
  mock.content = {
    patch: null,
    loading: false,
    error: new ApiError("failed", 500, {}) as unknown,
  };
  render(<HistoricalTurnDiffViewer target={{ ...target }} />);
  expect(screen.getByText("task:turnChangesContentUnavailable")).toBeTruthy();
  expect(screen.queryByText(LOADING_COPY)).toBeNull();
});
