import React from "react";
import { render, screen, cleanup, fireEvent } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { FileTreeNode } from "@/lib/types/backend";

vi.mock("@/components/toast-provider", () => ({
  useToast: () => ({ toast: vi.fn() }),
}));

import { FileContextMenu } from "./file-context-menu";

const FILE_NODE: FileTreeNode = { name: "README.md", path: "README.md", is_dir: false, size: 0 };
const DIR_NODE: FileTreeNode = { name: "src", path: "src", is_dir: true, size: 0 };

afterEach(cleanup);

function openMenu(triggerTestId: string) {
  const trigger = screen.getByTestId(triggerTestId);
  fireEvent.contextMenu(trigger);
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
