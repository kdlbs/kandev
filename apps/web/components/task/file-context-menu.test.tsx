import React, { act } from "react";
import { render, screen, cleanup, fireEvent, waitFor, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

const responsive = vi.hoisted(() => ({ isFinePointer: true }));

vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => responsive,
}));

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

// No `react-i18next` stub: `vitest.setup.ts` bootstraps a real i18next instance
// from the shipped catalogs, so these assertions exercise the copy a user
// actually sees. A key-echoing stub silently turns every migrated label into a
// raw `ns:key` and the assertions then only prove the stub works.

import { FileContextMenu, TreeNodeName, useFileRename } from "./file-context-menu";
import { FileTreeNodeTouchActions } from "./file-browser-parts";

const FILE_NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const DIR_NODE: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0 };
const DELETE_CONFIRM_POPOVER_ID = "file-delete-confirm-popover";
const RENAME_ROW = "rename-row";
const FOCUS_ANCHOR = "focus-anchor";
const BULK_TREE: FileTreeNode = {
  name: "root",
  path: "",
  is_dir: true,
  size: 0,
  children: [
    { name: "a.txt", path: "a.txt", is_dir: false, size: 1 },
    { name: "b.txt", path: "b.txt", is_dir: false, size: 1 },
  ],
};

afterEach(() => {
  cleanup();
  responsive.isFinePointer = true;
  vi.useRealTimers();
});

function openMenu(triggerTestId: string) {
  const trigger = screen.getByTestId(triggerTestId);
  fireEvent.contextMenu(trigger);
}

function BulkDeleteHarness({ onDeleteFile }: { onDeleteFile: (path: string) => Promise<boolean> }) {
  const [tree, setTree] = React.useState<FileTreeNode | null>(BULK_TREE);
  return (
    <>
      <FileContextMenu
        node={BULK_TREE}
        tree={tree}
        setTree={setTree}
        onDeleteFile={onDeleteFile}
        onStartRename={() => {}}
        selectedCount={2}
        selectedPaths={new Set(["a.txt", "b.txt"])}
      >
        <div data-testid="bulk-row">row</div>
      </FileContextMenu>
      <output data-testid="tree-paths">
        {tree?.children?.map((child) => child.path).join(",")}
      </output>
    </>
  );
}

function RenameHarness() {
  const [tree, setTree] = React.useState<FileTreeNode | null>(FILE_NODE);
  const rename = useFileRename(FILE_NODE, tree, setTree, vi.fn().mockResolvedValue(true));

  return (
    <FileContextMenu
      node={FILE_NODE}
      tree={tree}
      setTree={setTree}
      onRenameFile={vi.fn().mockResolvedValue(true)}
      onStartRename={rename.handleStartRename}
    >
      <div data-testid={RENAME_ROW}>
        <TreeNodeName node={FILE_NODE} isActive={false} gitStatus={undefined} rename={rename} />
      </div>
    </FileContextMenu>
  );
}

// `FileContextMenuSurface` defers `onStartRename` to `onCloseAutoFocus` and calls
// `preventDefault()` there so Radix does not pull focus back to the trigger and
// steal it from the freshly mounted input. Both halves of that are only correct
// if the deferral is scoped to a *pending* rename: any other menu item, and any
// dismissal, must leave Radix's default focus restoration alone and must never
// enter edit mode. A pending flag that outlives its close is the failure mode.
// The anchor stands in for whatever held focus when the user opened the menu.
// Radix restores focus to it on close, so asserting against it distinguishes a
// real restoration from focus falling back to `document.body`.
function PendingRenameHarness({ onStartRename }: { onStartRename: () => void }) {
  const [tree, setTree] = React.useState<FileTreeNode | null>(FILE_NODE);
  return (
    <>
      <button data-testid={FOCUS_ANCHOR}>anchor</button>
      <FileContextMenu
        node={FILE_NODE}
        tree={tree}
        setTree={setTree}
        onRenameFile={vi.fn().mockResolvedValue(true)}
        onDownloadFile={vi.fn().mockResolvedValue(true)}
        onStartRename={onStartRename}
      >
        <div data-testid={RENAME_ROW}>row</div>
      </FileContextMenu>
    </>
  );
}

describe("FileContextMenu rename", () => {
  it("does not start rename when a different menu item is selected", async () => {
    const onStartRename = vi.fn();
    render(<PendingRenameHarness onStartRename={onStartRename} />);

    openMenu(RENAME_ROW);
    fireEvent.click(screen.getByText("Download"));

    await waitFor(() => expect(screen.queryByText("Download")).toBeNull());
    expect(onStartRename).not.toHaveBeenCalled();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("does not start rename when the menu is dismissed without a selection", async () => {
    const onStartRename = vi.fn();
    render(<PendingRenameHarness onStartRename={onStartRename} />);

    openMenu(RENAME_ROW);
    fireEvent.keyDown(document, { key: "Escape" });

    await waitFor(() => expect(screen.queryByText("Rename")).toBeNull());
    expect(onStartRename).not.toHaveBeenCalled();
  });

  it("does not replay a consumed rename on the next menu close", async () => {
    const onStartRename = vi.fn();
    render(<PendingRenameHarness onStartRename={onStartRename} />);

    openMenu(RENAME_ROW);
    fireEvent.click(screen.getByText("Rename"));
    await waitFor(() => expect(onStartRename).toHaveBeenCalledTimes(1));

    // Reopening and dismissing must not re-fire the rename the previous close
    // already consumed.
    openMenu(RENAME_ROW);
    fireEvent.keyDown(document, { key: "Escape" });

    await waitFor(() => expect(screen.queryByText("Rename")).toBeNull());
    expect(onStartRename).toHaveBeenCalledTimes(1);
  });

  // The three tests above prove `onStartRename` stays unfired, which is only
  // half of the contract. `preventDefault()` suppresses Radix's focus
  // restoration, and hoisting it above the pending-rename guard would keep every
  // assertion above green while silently dropping the user back on
  // `document.body` after Download or Escape. These two pin the other half.
  it("restores focus to the opener when a different menu item is selected", async () => {
    render(<PendingRenameHarness onStartRename={vi.fn()} />);
    const anchor = screen.getByTestId(FOCUS_ANCHOR);
    anchor.focus();

    openMenu(RENAME_ROW);
    fireEvent.click(screen.getByText("Download"));

    await waitFor(() => expect(document.activeElement).toBe(anchor));
  });

  it("restores focus to the opener when the menu is dismissed without a selection", async () => {
    render(<PendingRenameHarness onStartRename={vi.fn()} />);
    const anchor = screen.getByTestId(FOCUS_ANCHOR);
    anchor.focus();

    openMenu(RENAME_ROW);
    fireEvent.keyDown(document, { key: "Escape" });

    await waitFor(() => expect(document.activeElement).toBe(anchor));
  });

  it("starts rename after the menu closes and immediately focuses the selected filename", async () => {
    vi.useFakeTimers();
    render(<RenameHarness />);

    openMenu(RENAME_ROW);
    fireEvent.click(screen.getByText("Rename"));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });

    const input = screen.getByRole("textbox");
    expect(document.activeElement).toBe(input);
    expect((input as HTMLInputElement).selectionStart).toBe(0);
    expect((input as HTMLInputElement).selectionEnd).toBe(FILE_NODE.name.length);
  });
});

describe("FileContextMenu Download item", () => {
  it("shows a Download item for a file when onDownloadFile is provided", () => {
    const onDownloadFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={FILE_NODE}
        tree={null}
        setTree={() => {}}
        onDownloadFile={onDownloadFile}
        onStartRename={() => {}}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>,
    );

    openMenu("file-row");
    const item = screen.getByText("Download");
    expect(item).toBeTruthy();
  });

  it("calls onDownloadFile with the node path when Download is selected", () => {
    const onDownloadFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={FILE_NODE}
        tree={null}
        setTree={() => {}}
        onDownloadFile={onDownloadFile}
        onStartRename={() => {}}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>,
    );

    openMenu("file-row");
    fireEvent.click(screen.getByText("Download"));

    expect(onDownloadFile).toHaveBeenCalledWith("README.md");
  });

  it("does not show a Download item for directories", () => {
    const onDownloadFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={DIR_NODE}
        tree={null}
        setTree={() => {}}
        onDeleteFile={vi.fn().mockResolvedValue(true)}
        onDownloadFile={onDownloadFile}
        onStartRename={() => {}}
      >
        <div data-testid="dir-row">row</div>
      </FileContextMenu>,
    );

    openMenu("dir-row");
    expect(screen.queryByText("Download")).toBeNull();
  });

  it("does not show a Download item when a bulk selection is active", () => {
    const onDownloadFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={FILE_NODE}
        tree={null}
        setTree={() => {}}
        onDeleteFile={vi.fn().mockResolvedValue(true)}
        onDownloadFile={onDownloadFile}
        onStartRename={() => {}}
        selectedCount={3}
        selectedPaths={new Set(["a", "b", "c"])}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>,
    );

    openMenu("file-row");
    expect(screen.queryByText("Download")).toBeNull();
  });
});

describe("FileContextMenu chat context item", () => {
  it.each([
    ["file", FILE_NODE],
    ["directory", DIR_NODE],
  ] as const)("shows and selects Add to chat context for a single %s", (_kind, node) => {
    const onAddToChatContext = vi.fn();
    render(
      <FileContextMenu
        node={node}
        tree={null}
        setTree={() => {}}
        onAddToChatContext={onAddToChatContext}
        onStartRename={() => {}}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>,
    );

    openMenu("file-row");
    const item = screen.getByTestId("file-context-add-to-chat");
    fireEvent.click(item);

    expect(onAddToChatContext).toHaveBeenCalledTimes(1);
    expect(onAddToChatContext).toHaveBeenCalledWith(node);
  });

  it("hides Add to chat context for bulk selections", () => {
    render(
      <FileContextMenu
        node={FILE_NODE}
        tree={null}
        setTree={() => {}}
        onDeleteFile={vi.fn().mockResolvedValue(true)}
        onAddToChatContext={vi.fn()}
        onStartRename={() => {}}
        selectedCount={2}
        selectedPaths={new Set(["README.md", "src"])}
      >
        <div data-testid="file-row">row</div>
      </FileContextMenu>,
    );

    openMenu("file-row");

    expect(screen.queryByTestId("file-context-add-to-chat")).toBeNull();
  });
});

describe("FileContextMenu bulk deletion", () => {
  it.each([
    ["file", FILE_NODE],
    ["folder", DIR_NODE],
  ] as const)("uses local confirmation for one %s", async (_kind, node) => {
    const onDeleteFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={node}
        tree={node}
        setTree={() => {}}
        onDeleteFile={onDeleteFile}
        onStartRename={() => {}}
      >
        <div data-testid="single-delete-row">row</div>
      </FileContextMenu>,
    );

    openMenu("single-delete-row");
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));

    expect(onDeleteFile).not.toHaveBeenCalled();
    expect(screen.queryByRole("alertdialog")).toBeNull();
    await waitFor(() => expect(screen.getByTestId(DELETE_CONFIRM_POPOVER_ID)).toBeTruthy());
    const confirmation = screen.getByTestId(DELETE_CONFIRM_POPOVER_ID);
    if (node.is_dir) {
      expect(confirmation.textContent).toContain("This will permanently delete src");
    } else {
      expect(confirmation.textContent).toContain("This will permanently delete README.md");
      expect(confirmation.textContent).not.toContain("file inside it");
    }

    fireEvent.click(screen.getByRole("button", { name: "Cancel" }));
    await waitFor(() => expect(screen.queryByTestId(DELETE_CONFIRM_POPOVER_ID)).toBeNull());
    expect(onDeleteFile).not.toHaveBeenCalled();

    openMenu("single-delete-row");
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete" }));
    await waitFor(() => expect(screen.getByTestId("file-delete-confirm")).toBeTruthy());
    fireEvent.click(screen.getByTestId("file-delete-confirm"));

    await waitFor(() => expect(onDeleteFile).toHaveBeenCalledWith(node.path));
  });

  it("keeps a multi-selection in the scope-explaining modal", () => {
    const onDeleteFile = vi.fn().mockResolvedValue(true);
    render(<BulkDeleteHarness onDeleteFile={onDeleteFile} />);

    openMenu("bulk-row");
    fireEvent.click(screen.getByRole("menuitem", { name: "Delete 2 items" }));

    expect(screen.getByRole("alertdialog")).toBeTruthy();
    expect(screen.queryByTestId(DELETE_CONFIRM_POPOVER_ID)).toBeNull();
  });

  it("keeps failed paths visible after a partial deletion failure", async () => {
    const onDeleteFile = vi.fn(async (path: string) => path === "a.txt");
    render(<BulkDeleteHarness onDeleteFile={onDeleteFile} />);

    openMenu("bulk-row");
    fireEvent.click(screen.getByText("Delete 2 items"));
    fireEvent.click(screen.getByRole("button", { name: "Delete" }));

    await waitFor(() => expect(onDeleteFile).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(screen.getByTestId("tree-paths").textContent).toBe("b.txt"));
    expect(screen.getByTestId("tree-paths").textContent).not.toContain("a.txt");
  });
});

describe("FileContextMenu touch actions", () => {
  it("exposes Delete in the touch menu and replaces it with 44px inline actions", async () => {
    responsive.isFinePointer = false;
    const onDeleteFile = vi.fn().mockResolvedValue(true);
    render(
      <FileContextMenu
        node={FILE_NODE}
        tree={FILE_NODE}
        setTree={() => {}}
        onDeleteFile={onDeleteFile}
        onStartRename={() => {}}
      >
        <div data-testid="touch-delete-row">
          <FileTreeNodeTouchActions node={FILE_NODE} showTouchActions />
        </div>
      </FileContextMenu>,
    );

    fireEvent.pointerDown(screen.getByTestId("file-tree-node-actions"));
    fireEvent.click(screen.getByTestId("file-tree-touch-delete"));

    await waitFor(() => expect(screen.getByTestId("file-delete-inline-confirmation")).toBeTruthy());
    expect(screen.queryByTestId(DELETE_CONFIRM_POPOVER_ID)).toBeNull();
    expect(onDeleteFile).not.toHaveBeenCalled();
    expect(
      screen.getByTestId("file-delete-inline-confirmation").querySelectorAll("button"),
    ).toHaveLength(2);

    fireEvent.click(
      within(screen.getByTestId("file-delete-inline-confirmation")).getByTestId(
        "file-delete-confirm",
      ),
    );
    await waitFor(() => expect(onDeleteFile).toHaveBeenCalledWith(FILE_NODE.path));
  });
});

describe("FileContextMenu upload guard", () => {
  const UPLOAD_LABEL = "Upload files here";
  const ROW = "upload-guard-row";

  function renderFor(node: FileTreeNode, selectedCount = 1) {
    const onUploadFilesHere = vi.fn();
    render(
      <FileContextMenu
        node={node}
        tree={node}
        setTree={vi.fn()}
        onDeleteFile={vi.fn().mockResolvedValue(true)}
        onUploadFilesHere={onUploadFilesHere}
        onStartRename={vi.fn()}
        selectedCount={selectedCount}
        selectedPaths={selectedCount > 1 ? new Set(["a.txt", "b.txt"]) : undefined}
      >
        <div data-testid={ROW}>row</div>
      </FileContextMenu>,
    );
    openMenu(ROW);
    return { onUploadFilesHere };
  }

  it("offers upload on a folder and targets that folder", async () => {
    const { onUploadFilesHere } = renderFor(DIR_NODE);

    const item = await screen.findByRole("menuitem", { name: UPLOAD_LABEL });
    fireEvent.click(item);
    await waitFor(() => expect(onUploadFilesHere).toHaveBeenCalledWith("src"));
  });

  it("is absent for a file, the exact inverse of the download guard", async () => {
    renderFor(FILE_NODE);

    await screen.findByRole("menuitem", { name: "Delete" });
    expect(screen.queryByRole("menuitem", { name: UPLOAD_LABEL })).toBeNull();
  });

  it("is absent for a multi-selection, which has no single destination", async () => {
    renderFor(DIR_NODE, 2);

    await waitFor(() => expect(screen.queryAllByRole("menuitem").length).toBeGreaterThan(0));
    expect(screen.queryByRole("menuitem", { name: UPLOAD_LABEL })).toBeNull();
  });
});
