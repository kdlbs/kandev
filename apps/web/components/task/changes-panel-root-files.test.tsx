import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { WorkspaceRootFilesResponse } from "@/lib/types/workspace-files";
import { ChangesPanelRootFiles } from "./changes-panel-root-files";

const ROOT_FILE_PATH = "dashboard.json";
const NESTED_FILE_PATH = "reports/daily/summary.md";
const NESTED_FOLDER_PATH = "reports/daily";
const NESTED_LOG_PATH = "reports/daily/log.json";
const REPORTS_PATH = "reports";
const WEEKLY_FILE_PATH = "reports/weekly/summary.md";
const COLLIDING_DIRECTORY_PATH = "reports/summary.md";
const COLLIDING_ROOT_FILE_PATH = "directory:reports";
const OUTSIDE_GROUP_NAME = /Outside repositories/;
const ROOT_FILES_SECTION_TEST_ID = "changes-workspace-root-files";
const ARIA_EXPANDED_ATTRIBUTE = "aria-expanded";
const EXPANDED = "true";
const COLLAPSED = "false";
const CONTEXT_A = "task-a:environment-a";
const CONTEXT_B = "task-b:environment-b";
const MODIFIED_AT = "2026-11-19T10:00:00Z";

const listing: WorkspaceRootFilesResponse = {
  supported: true,
  files: [{ path: ROOT_FILE_PATH, size: 12, modified_at: MODIFIED_AT }],
  truncated: false,
  destinations: [{ repository_name: "web", path: "apps/web" }],
};

const nestedListing: WorkspaceRootFilesResponse = {
  ...listing,
  files: [
    { path: NESTED_FILE_PATH, size: 12, modified_at: MODIFIED_AT },
    { path: "notes.md", size: 8, modified_at: MODIFIED_AT },
  ],
};

const mocks = vi.hoisted(() => ({
  renameFile: vi.fn(),
  toast: vi.fn(),
  layout: "tree",
  isMobile: false,
  isFinePointer: true,
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: { userSettings: { changesPanelLayout: string } }) => unknown) =>
    selector({ userSettings: { changesPanelLayout: mocks.layout } }),
}));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({
    isMobile: mocks.isMobile,
    isFinePointer: mocks.isFinePointer,
  }),
}));

vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => ({}) }));
vi.mock("@/lib/ws/workspace-files", () => ({ renameFile: mocks.renameFile }));
vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: mocks.toast }),
}));

afterEach(cleanup);
beforeEach(() => {
  mocks.layout = "tree";
  mocks.isMobile = false;
  mocks.isFinePointer = true;
});

function renderRootFiles({
  files = listing,
  contextKey = CONTEXT_A,
  sessionId = "session-1",
  onEditFile = vi.fn(),
}: {
  files?: WorkspaceRootFilesResponse | undefined;
  contextKey?: string;
  sessionId?: string | null;
  onEditFile?: (path: string, repo?: string) => void;
} = {}) {
  return render(
    <ChangesPanelRootFiles
      contextKey={contextKey}
      listing={files}
      sessionId={sessionId}
      onEditFile={onEditFile}
    />,
  );
}

it("hides unsupported and empty listings", () => {
  const view = renderRootFiles({ files: { ...listing, supported: false } });
  expect(screen.queryByTestId(ROOT_FILES_SECTION_TEST_ID)).toBeNull();

  view.rerender(
    <ChangesPanelRootFiles
      contextKey={CONTEXT_A}
      listing={{ ...listing, files: [] }}
      sessionId="session-1"
      onEditFile={vi.fn()}
    />,
  );
  expect(screen.queryByTestId(ROOT_FILES_SECTION_TEST_ID)).toBeNull();
});

it("starts expanded and preserves disclosure choices across file refreshes", () => {
  const view = renderRootFiles({ files: nestedListing });
  const group = screen.getByRole("button", { name: OUTSIDE_GROUP_NAME });
  const folder = screen.getByRole("button", { name: NESTED_FOLDER_PATH });
  expect(group.getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe(EXPANDED);
  expect(screen.getByRole("button", { name: NESTED_FILE_PATH })).toBeTruthy();

  fireEvent.click(folder);
  expect(screen.queryByRole("button", { name: NESTED_FILE_PATH })).toBeNull();

  view.rerender(
    <ChangesPanelRootFiles
      contextKey={CONTEXT_A}
      listing={{
        ...nestedListing,
        files: [
          ...nestedListing.files,
          {
            path: NESTED_LOG_PATH,
            size: 4,
            modified_at: MODIFIED_AT,
          },
        ],
      }}
      sessionId="session-1"
      onEditFile={vi.fn()}
    />,
  );
  expect(
    screen.getByRole("button", { name: NESTED_FOLDER_PATH }).getAttribute(ARIA_EXPANDED_ATTRIBUTE),
  ).toBe(COLLAPSED);

  fireEvent.click(screen.getByRole("button", { name: OUTSIDE_GROUP_NAME }));
  expect(
    screen.getByRole("button", { name: OUTSIDE_GROUP_NAME }).getAttribute(ARIA_EXPANDED_ATTRIBUTE),
  ).toBe(COLLAPSED);
  expect(screen.getByTestId(ROOT_FILES_SECTION_TEST_ID).textContent).toContain("3");
  fireEvent.click(screen.getByRole("button", { name: OUTSIDE_GROUP_NAME }));
  expect(
    screen.getByRole("button", { name: NESTED_FOLDER_PATH }).getAttribute(ARIA_EXPANDED_ATTRIBUTE),
  ).toBe(COLLAPSED);
});

it("keeps a collapsed directory as a boundary when a refresh makes its chain compactable", () => {
  const view = renderRootFiles({
    files: {
      ...listing,
      files: [
        { path: NESTED_FILE_PATH, size: 12, modified_at: MODIFIED_AT },
        { path: WEEKLY_FILE_PATH, size: 8, modified_at: MODIFIED_AT },
      ],
    },
  });
  fireEvent.click(screen.getByRole("button", { name: REPORTS_PATH }));

  view.rerender(
    <ChangesPanelRootFiles
      contextKey={CONTEXT_A}
      listing={{
        ...listing,
        files: [{ path: NESTED_FILE_PATH, size: 12, modified_at: MODIFIED_AT }],
      }}
      sessionId="session-1"
      onEditFile={vi.fn()}
    />,
  );

  const collapsedReports = screen.getByRole("button", { name: REPORTS_PATH });
  expect(collapsedReports.getAttribute(ARIA_EXPANDED_ATTRIBUTE)).toBe(COLLAPSED);
  expect(screen.queryByRole("button", { name: NESTED_FILE_PATH })).toBeNull();

  fireEvent.click(collapsedReports);
  expect(screen.getByRole("button", { name: NESTED_FOLDER_PATH })).toBeTruthy();
  expect(screen.getByRole("button", { name: NESTED_FILE_PATH })).toBeTruthy();
});

it("keeps directory and file rows distinct across a listing refresh", () => {
  const consoleError = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const view = renderRootFiles({
      files: {
        ...listing,
        files: [
          { path: COLLIDING_DIRECTORY_PATH, size: 8, modified_at: MODIFIED_AT },
          { path: COLLIDING_ROOT_FILE_PATH, size: 12, modified_at: MODIFIED_AT },
        ],
      },
    });
    view.rerender(
      <ChangesPanelRootFiles
        contextKey={CONTEXT_A}
        listing={{
          ...listing,
          files: [
            { path: COLLIDING_DIRECTORY_PATH, size: 8, modified_at: MODIFIED_AT },
            { path: COLLIDING_ROOT_FILE_PATH, size: 12, modified_at: MODIFIED_AT },
            { path: "extra.md", size: 4, modified_at: MODIFIED_AT },
          ],
        }}
        sessionId="session-1"
        onEditFile={vi.fn()}
      />,
    );

    expect(screen.getByRole("button", { name: REPORTS_PATH })).toBeTruthy();
    expect(screen.getByRole("button", { name: COLLIDING_ROOT_FILE_PATH })).toBeTruthy();
    expect(
      consoleError.mock.calls.some((args) => args.map(String).join(" ").includes("same key")),
    ).toBe(false);
  } finally {
    consoleError.mockRestore();
  }
});

it("resets disclosure choices when the Changes context changes", () => {
  const view = renderRootFiles({ files: nestedListing });
  fireEvent.click(screen.getByRole("button", { name: NESTED_FOLDER_PATH }));
  fireEvent.click(screen.getByRole("button", { name: OUTSIDE_GROUP_NAME }));

  view.rerender(
    <ChangesPanelRootFiles
      contextKey={CONTEXT_B}
      listing={nestedListing}
      sessionId="session-2"
      onEditFile={vi.fn()}
    />,
  );
  expect(
    screen.getByRole("button", { name: OUTSIDE_GROUP_NAME }).getAttribute(ARIA_EXPANDED_ATTRIBUTE),
  ).toBe(EXPANDED);
  expect(
    screen.getByRole("button", { name: NESTED_FOLDER_PATH }).getAttribute(ARIA_EXPANDED_ATTRIBUTE),
  ).toBe(EXPANDED);
  expect(screen.getByRole("button", { name: NESTED_FILE_PATH })).toBeTruthy();
});

it("uses the flat Changes layout to show root-relative paths", () => {
  mocks.layout = "flat";
  renderRootFiles({ files: nestedListing });

  expect(screen.getByRole("button", { name: NESTED_FILE_PATH })).toBeTruthy();
  expect(screen.queryByRole("button", { name: NESTED_FOLDER_PATH })).toBeNull();
});

it("shows the truncation note and opens a file without a repository scope", () => {
  const onEditFile = vi.fn();
  renderRootFiles({ files: { ...listing, truncated: true }, onEditFile });

  expect(screen.getByRole("button", { name: OUTSIDE_GROUP_NAME })).toBeTruthy();
  expect(screen.getByTestId(ROOT_FILES_SECTION_TEST_ID).textContent).toContain("1");
  expect(screen.getByText("Some workspace root files are not shown.")).toBeTruthy();
  fireEvent.click(screen.getByRole("button", { name: ROOT_FILE_PATH }));
  expect(onEditFile).toHaveBeenCalledWith(ROOT_FILE_PATH, undefined);
});

it("offers the move action when a destination repository is available", async () => {
  renderRootFiles();
  fireEvent.pointerDown(screen.getByRole("button", { name: "Show more actions" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() => {
    expect(screen.getByRole("menuitem", { name: "Move to repository..." })).toBeTruthy();
  });
});

it("closes the move dialog when the session changes", async () => {
  const view = renderRootFiles();
  fireEvent.pointerDown(screen.getByRole("button", { name: "Show more actions" }), {
    button: 0,
    ctrlKey: false,
  });
  await waitFor(() =>
    expect(screen.getByRole("menuitem", { name: "Move to repository..." })).toBeTruthy(),
  );
  fireEvent.click(screen.getByRole("menuitem", { name: "Move to repository..." }));
  expect(screen.getByRole("dialog")).toBeTruthy();

  view.rerender(
    <ChangesPanelRootFiles
      contextKey={CONTEXT_A}
      listing={listing}
      sessionId="session-2"
      onEditFile={vi.fn()}
    />,
  );
  expect(screen.queryByRole("dialog")).toBeNull();
});
