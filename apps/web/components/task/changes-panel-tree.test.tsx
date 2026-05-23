import { describe, it, expect, vi, afterEach } from "vitest";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import type { ComponentProps } from "react";

vi.mock("./changes-panel-file-row", () => ({
  FileRow: ({ file, indentPx }: { file: { path: string }; indentPx?: number }) => (
    <li data-testid="file-row" data-indent={indentPx ?? 0}>
      {file.path}
    </li>
  ),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => null,
}));

import { ChangesTree } from "./changes-panel-tree";

afterEach(cleanup);

type Props = ComponentProps<typeof ChangesTree>;

const stubMultiSelect: Props["multiSelect"] = {
  selectedPaths: new Set<string>(),
  isSelected: () => false,
  handleClick: vi.fn(() => false),
  clearSelection: vi.fn(),
  selectAll: vi.fn(),
  setSelectedPaths: vi.fn(),
};

const baseProps: Omit<Props, "files" | "variant"> = {
  pendingStageFiles: new Set(),
  onOpenDiff: vi.fn(),
  onEditFile: vi.fn(),
  onStage: vi.fn(),
  onUnstage: vi.fn(),
  onDiscard: vi.fn(),
  multiSelect: stubMultiSelect,
};

function file(path: string): Props["files"][number] {
  return {
    path,
    status: "modified",
    staged: false,
    plus: 1,
    minus: 0,
    oldPath: undefined,
  };
}

const APPS_WEB_DIR = "tree-dir-apps-web";
const FOO_TS = "apps/web/foo.ts";
const BAR_TS = "apps/web/bar.ts";

describe("ChangesTree", () => {
  it("renders folders as dir rows and files as file rows", () => {
    render(
      <ChangesTree
        {...baseProps}
        variant="unstaged"
        files={[file(FOO_TS), file(BAR_TS), file("README.md")]}
      />,
    );
    // Two file rows under apps/web plus README at root.
    expect(screen.getAllByTestId("file-row")).toHaveLength(3);
    // A directory row exists for "apps/web" (chain-collapsed).
    const dir = screen.getByTestId(APPS_WEB_DIR);
    expect(dir.textContent).toContain("apps/web");
  });

  it("collapses a single-child dir chain into one row", () => {
    render(<ChangesTree {...baseProps} variant="unstaged" files={[file("a/b/c/file.ts")]} />);
    // The chain a/b/c renders as a single dir row keyed by the deepest path.
    const dir = screen.getByTestId("tree-dir-a-b-c");
    expect(dir.textContent).toContain("a/b/c");
  });

  it("hides children when a folder is collapsed", () => {
    render(<ChangesTree {...baseProps} variant="unstaged" files={[file(FOO_TS), file(BAR_TS)]} />);
    expect(screen.getAllByTestId("file-row")).toHaveLength(2);
    fireEvent.click(screen.getByTestId(APPS_WEB_DIR));
    expect(screen.queryAllByTestId("file-row")).toHaveLength(0);
  });

  it("uses the parent multiSelect (not an internal one) so bulk actions stay wired", () => {
    // Regression: tree mode used to instantiate its own useMultiSelect, which
    // left the section-level BulkActionBar blind to tree selections. Verify
    // the prop is consulted by checking that isSelected/handleClick come from
    // the supplied object.
    const isSelected = vi.fn(() => true);
    const handleClick = vi.fn(() => false);
    render(
      <ChangesTree
        {...baseProps}
        multiSelect={{ ...stubMultiSelect, isSelected, handleClick }}
        variant="unstaged"
        files={[file(FOO_TS)]}
      />,
    );
    // Our FileRow mock surfaces isSelected via a data attribute when wired.
    // Even without the attribute, the spy proves the parent's hook ran for
    // this file's path during render.
    expect(isSelected).toHaveBeenCalledWith(FOO_TS);
  });
});
