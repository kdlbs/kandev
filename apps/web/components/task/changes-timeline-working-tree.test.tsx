import { Fragment, type ReactNode } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { ChangedFile } from "./changes-panel-helpers";
import { ChangesWorkingTree } from "./changes-timeline-working-tree";

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { userSettings: { changesPanelLayout: string } }) => unknown) =>
    selector({ userSettings: { changesPanelLayout: "tree" } }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: (selector: (state: { activeFilePath: string | null }) => unknown) =>
    selector({ activeFilePath: null }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: false, isFinePointer: true }),
}));

vi.mock("@/hooks/use-copy-repository-path", () => ({
  useCopyRepositoryPath: () => vi.fn(),
}));

vi.mock("./changes-timeline-viewport", () => ({
  ChangesTimelineViewport: ({
    rows,
    renderRow,
  }: {
    rows: Array<{ key: string }>;
    renderRow: (row: { key: string }, index: number) => ReactNode;
  }) => (
    <div data-testid="mock-timeline-viewport">
      {rows.map((row, index) => (
        <Fragment key={row.key}>{renderRow(row, index)}</Fragment>
      ))}
    </div>
  ),
}));

afterEach(cleanup);

const TRUE_ATTRIBUTE = "true";
const NOT_SELECTED = "false";
const TASK_A_ENV_A = JSON.stringify(["task-a", "session-a", "environment-a"]);
const TASK_B_ENV_B = JSON.stringify(["task-b", "session-b", "environment-b"]);
const TASK_A_ENV_B = JSON.stringify(["task-a", "session-a", "environment-b"]);
const FRONTEND_FILE_TEST_ID = "file-row-src-shared.ts";
const DATA_SELECTED = "data-selected";

const sharedFiles: ChangedFile[] = [
  {
    path: "src/shared.ts",
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
    repositoryName: "frontend",
  },
  {
    path: "lib/shared.ts",
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
    repositoryName: "backend",
  },
];

function WorkingTreeOwner({ contextKey }: { contextKey: string }) {
  const noop = () => {};
  return (
    <TooltipProvider>
      <ChangesWorkingTree
        hasUnstaged
        hasStaged={false}
        unstagedFiles={sharedFiles}
        stagedFiles={[]}
        pendingStageFiles={new Set()}
        onOpenDiff={noop}
        onEditFile={noop}
        onStage={noop}
        onUnstage={noop}
        onDiscard={noop}
        onBulkStage={noop}
        onBulkUnstage={noop}
        onBulkDiscard={noop}
        repoDisplayName={(repositoryName) => repositoryName}
        isLoading={false}
        scrollElement={null}
        beforeLayoutKey="stable-layout"
        contextKey={contextKey}
      />
    </TooltipProvider>
  );
}

function repositoryToggle(repositoryName: string): HTMLElement {
  const toggle = screen
    .getAllByTestId("changes-repo-header")
    .find((element) => element.textContent?.includes(repositoryName));
  if (!toggle) throw new Error(`Missing ${repositoryName} repository toggle`);
  return toggle;
}

function expectFreshWorkingTree() {
  expect(
    screen.getByTestId("unstaged-files-section-collapse-toggle").getAttribute("aria-expanded"),
  ).toBe(TRUE_ATTRIBUTE);
  for (const repository of screen.getAllByTestId("changes-repo-header")) {
    expect(repository.getAttribute("aria-expanded")).toBe(TRUE_ATTRIBUTE);
  }
  expect(screen.getByTestId("tree-dir-src").getAttribute("aria-expanded")).toBe(TRUE_ATTRIBUTE);
  expect(screen.getByTestId(FRONTEND_FILE_TEST_ID).getAttribute(DATA_SELECTED)).toBe(NOT_SELECTED);
}

describe("ChangesWorkingTree context ownership", () => {
  it("resets selection anchors and every working-tree disclosure for task and environment changes", () => {
    const view = render(<WorkingTreeOwner contextKey={TASK_A_ENV_A} />);

    fireEvent.click(screen.getByTestId(FRONTEND_FILE_TEST_ID), { ctrlKey: true });
    expect(screen.getByTestId(FRONTEND_FILE_TEST_ID).getAttribute(DATA_SELECTED)).toBe(
      TRUE_ATTRIBUTE,
    );
    fireEvent.click(screen.getByTestId("tree-dir-src"));
    fireEvent.click(repositoryToggle("frontend"));
    fireEvent.click(screen.getByTestId("unstaged-files-section-collapse-toggle"));

    view.rerender(<WorkingTreeOwner contextKey={TASK_B_ENV_B} />);
    expectFreshWorkingTree();

    fireEvent.click(screen.getByTestId("file-row-lib-shared.ts"), { shiftKey: true });
    expect(screen.getByTestId("file-row-lib-shared.ts").getAttribute(DATA_SELECTED)).toBe(
      TRUE_ATTRIBUTE,
    );
    expect(screen.getByTestId(FRONTEND_FILE_TEST_ID).getAttribute(DATA_SELECTED)).toBe(
      NOT_SELECTED,
    );

    view.rerender(<WorkingTreeOwner contextKey={TASK_A_ENV_A} />);
    expectFreshWorkingTree();

    fireEvent.click(screen.getByTestId(FRONTEND_FILE_TEST_ID), { ctrlKey: true });
    fireEvent.click(screen.getByTestId("unstaged-files-section-collapse-toggle"));
    view.rerender(<WorkingTreeOwner contextKey={TASK_A_ENV_B} />);
    expectFreshWorkingTree();
  });
});
