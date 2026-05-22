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

vi.mock("@/hooks/use-multi-select", () => ({
  useMultiSelect: () => ({
    selectedPaths: new Set<string>(),
    isSelected: () => false,
    handleClick: vi.fn(),
    clearSelection: vi.fn(),
  }),
}));

vi.mock("@/lib/state/dockview-store", () => ({
  useDockviewStore: () => null,
}));

import { ChangesTree } from "./changes-panel-tree";

afterEach(cleanup);

type Props = ComponentProps<typeof ChangesTree>;

const baseProps: Omit<Props, "files" | "variant"> = {
  pendingStageFiles: new Set(),
  onOpenDiff: vi.fn(),
  onEditFile: vi.fn(),
  onStage: vi.fn(),
  onUnstage: vi.fn(),
  onDiscard: vi.fn(),
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

describe("ChangesTree", () => {
  it("renders folders as dir rows and files as file rows", () => {
    render(
      <ChangesTree
        {...baseProps}
        variant="unstaged"
        files={[file("apps/web/foo.ts"), file("apps/web/bar.ts"), file("README.md")]}
      />,
    );
    // Two file rows under apps/web plus README at root.
    expect(screen.getAllByTestId("file-row")).toHaveLength(3);
    // A directory row exists for "apps/web" (chain-collapsed).
    const dir = screen.getByTestId("tree-dir-apps-web");
    expect(dir.textContent).toContain("apps/web");
  });

  it("collapses a single-child dir chain into one row", () => {
    render(<ChangesTree {...baseProps} variant="unstaged" files={[file("a/b/c/file.ts")]} />);
    // The chain a/b/c renders as a single dir row keyed by the deepest path.
    const dir = screen.getByTestId("tree-dir-a-b-c");
    expect(dir.textContent).toContain("a/b/c");
  });

  it("hides children when a folder is collapsed", () => {
    render(
      <ChangesTree
        {...baseProps}
        variant="unstaged"
        files={[file("apps/web/foo.ts"), file("apps/web/bar.ts")]}
      />,
    );
    expect(screen.getAllByTestId("file-row")).toHaveLength(2);
    fireEvent.click(screen.getByTestId("tree-dir-apps-web"));
    expect(screen.queryAllByTestId("file-row")).toHaveLength(0);
  });
});
