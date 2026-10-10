import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { TurnChangeSetSummary } from "@/lib/types/turn-changes";
import { TurnChangedFilesCard } from "./turn-changed-files-card";

const sessionId = "session-1";
const repositoryChangeId = "repo-change-1";
const checkoutId = "checkout-1";

const mockFiles = vi.hoisted(() => ({
  byRepository: {} as Record<string, unknown[]>,
  totalsByRepository: {} as Record<string, number>,
}));

vi.mock("@/hooks/domains/session/use-turn-change-files", () => ({
  useTurnChangeFiles: () => ({
    filesByRepository: mockFiles.byRepository,
    fileTotalsByRepository: mockFiles.totalsByRepository,
    loading: false,
    error: null,
    hasMore: false,
    loadMore: vi.fn(),
  }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: translateCardTestCopy,
  }),
}));

vi.mock("@kandev/ui/button", () => ({
  Button: ({ children, ...props }: React.ButtonHTMLAttributes<HTMLButtonElement>) => (
    <button {...props}>{children}</button>
  ),
}));

afterEach(() => {
  cleanup();
  mockFiles.byRepository = {};
  mockFiles.totalsByRepository = {};
});

const summary: TurnChangeSetSummary = {
  id: "set-1",
  task_id: "task-1",
  session_id: sessionId,
  turn_id: "turn-1",
  revision: 2,
  availability: "ready",
  complete: true,
  summary_complete: true,
  content_complete: true,
  turn_ordinal: 1,
  final_assistant_message_id: "message-1",
  fallback_anchor: "turn-changes:turn-1",
  file_count: 1,
  binary_file_count: 0,
  unknown_count_file_count: 0,
  repository_count: 1,
  repositories: [
    {
      id: repositoryChangeId,
      checkout_id: checkoutId,
      display_name: "app",
      availability: "ready",
      enumeration_complete: true,
      comparison_complete: true,
      content_complete: true,
    },
  ],
};

function translateCardTestCopy(key: string, options?: Record<string, unknown>): string {
  const translations: Record<string, () => string> = {
    "task:turnChangesFileCount": () => `${options?.count ?? 0} changed files`,
    "task:turnChangesLoadedFileCount": () => `${options?.count ?? 0} loaded files`,
    "task:turnChangesFolderAccessibleLabel": () =>
      `${options?.path} folder, ${options?.count} loaded files, +${options?.added} -${options?.deleted}, ${options?.unknown} unknown`,
    "task:turnChangesFileAccessibleLabel": () =>
      `${options?.path} ${options?.kind} ${options?.status} ${options?.mode}`,
    "task:turnChangesModeStatus": () => `Mode changed ${options?.oldMode} to ${options?.newMode}`,
    "task:turnChangesRepositoryStatus": () => `Repository status: ${options?.status}`,
  };
  return translations[key]?.() ?? key;
}

it("keeps folder expansion separate from exact checkout file selection", () => {
  mockFiles.byRepository = {
    [repositoryChangeId]: [
      {
        id: "file-1",
        repository_change_id: repositoryChangeId,
        checkout_id: checkoutId,
        path: "src/main.ts",
        kind: "modified",
        added_lines: 3,
        deleted_lines: 1,
        content_availability: "ready",
      },
    ],
  };
  mockFiles.totalsByRepository = { [repositoryChangeId]: 131 };
  const onOpenDiff = vi.fn();
  render(<TurnChangedFilesCard sessionId={sessionId} summary={summary} onOpenDiff={onOpenDiff} />);

  fireEvent.click(screen.getByRole("button", { name: /src folder/ }));
  expect(onOpenDiff).not.toHaveBeenCalled();
  expect(screen.getByText("131 changed files")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: /src\/main\.ts/ }));
  expect(onOpenDiff).toHaveBeenCalledWith({
    sessionId,
    changeSetId: "set-1",
    repositoryChangeId,
    fileChangeId: "file-1",
    checkoutId,
    path: "src/main.ts",
    fileKind: "modified",
  });
});

it("labels renamed binary files without inventing line counts", () => {
  mockFiles.byRepository = {
    [repositoryChangeId]: [
      {
        id: "file-2",
        repository_change_id: repositoryChangeId,
        checkout_id: checkoutId,
        path: "assets/logo.png",
        old_path: "assets/old.png",
        kind: "renamed",
        binary: true,
        content_availability: "ready",
      },
    ],
  };
  render(<TurnChangedFilesCard sessionId={sessionId} summary={summary} onOpenDiff={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /assets folder/ }));
  expect(screen.getByText("task:turnChangesBinary")).toBeTruthy();
  expect(screen.getByText(/task:turnChangesRenamedFrom/)).toBeTruthy();
});

it("labels incomplete repository enumeration as a loaded subset", () => {
  mockFiles.byRepository = {
    [repositoryChangeId]: [
      {
        id: "file-one",
        repository_change_id: repositoryChangeId,
        checkout_id: checkoutId,
        path: "src/one.ts",
        kind: "added",
        added_lines: 2,
        deleted_lines: 0,
        content_availability: "ready",
      },
    ],
  };
  mockFiles.totalsByRepository = { [repositoryChangeId]: 100 };
  const partialSummary = {
    ...summary,
    repositories: [{ ...summary.repositories[0]!, enumeration_complete: false }],
  };
  render(
    <TurnChangedFilesCard sessionId={sessionId} summary={partialSummary} onOpenDiff={vi.fn()} />,
  );
  expect(screen.getAllByText("1 loaded files")).toHaveLength(2);
  expect(screen.queryByText("100 changed files")).toBeNull();
});

it("shows the shared-checkout attribution note when an overlap is known", () => {
  render(
    <TurnChangedFilesCard
      sessionId={sessionId}
      summary={{
        ...summary,
        overlap_intervals: [
          {
            change_set_id: "other-set",
            checkout_id: checkoutId,
            started_at: "2026-10-08T10:00:00Z",
          },
        ],
      }}
      onOpenDiff={vi.fn()}
    />,
  );
  expect(screen.getByRole("status").textContent).toContain("task:turnChangesSharedCheckout");
});

it("shows content loss for the affected repository in a partial turn", () => {
  const partialSummary = {
    ...summary,
    complete: false,
    content_complete: false,
    repositories: [{ ...summary.repositories[0]!, content_complete: false }],
  };
  render(
    <TurnChangedFilesCard sessionId={sessionId} summary={partialSummary} onOpenDiff={vi.fn()} />,
  );
  expect(screen.getByText(/task:turnChangesStatus_content_unavailable/)).toBeTruthy();
});

it("exposes mode-only changes in the file's accessible name", () => {
  mockFiles.byRepository = {
    [repositoryChangeId]: [
      {
        id: "mode-only",
        repository_change_id: repositoryChangeId,
        checkout_id: checkoutId,
        path: "bin/tool",
        kind: "mode_changed",
        old_mode: "100644",
        new_mode: "100755",
        added_lines: 0,
        deleted_lines: 0,
        content_availability: "ready",
      },
    ],
  };
  render(<TurnChangedFilesCard sessionId={sessionId} summary={summary} onOpenDiff={vi.fn()} />);
  fireEvent.click(screen.getByRole("button", { name: /bin folder/ }));
  expect(
    screen.getByRole("button", { name: /bin\/tool.*Mode changed 100644 to 100755/ }),
  ).toBeTruthy();
});
