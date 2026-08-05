import React from "react";
import { render, screen, cleanup, fireEvent, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string) => ({ "chat:addToChatContext": "Add to chat context" })[key] ?? key,
  }),
}));

import { FileContextMenu } from "./file-context-menu";

const FILE_NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const DIR_NODE: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0 };
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

afterEach(cleanup);

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
