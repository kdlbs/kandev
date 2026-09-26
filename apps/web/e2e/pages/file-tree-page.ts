import { expect, type Locator, type Page } from "@playwright/test";

/** Focused page object for Files tree and composer context selectors. */
export class FileTreePage {
  constructor(
    private readonly page: Page,
    private readonly files: Locator,
    private readonly activeChat: () => Locator,
  ) {}

  /** Find a tree node by its data-path attribute. */
  fileTreeNode(nodePath: string): Locator {
    return this.page.locator(
      `[data-testid="file-tree-node"][data-path=${JSON.stringify(nodePath)}]:visible`,
    );
  }

  /**
   * Wait for a tree row and reveal it when the virtualizer has mounted it
   * below the current viewport. The initial direct check keeps common rows
   * instant, while the bounded sweep handles large workspaces deterministically.
   */
  async waitForFileTreeNode(nodePath: string, timeout = 30_000): Promise<Locator> {
    const node = this.fileTreeNode(nodePath);
    if (await node.isVisible().catch(() => false)) return node;

    const viewport = this.fileTreeScrollViewport();
    await viewport.waitFor({ state: "visible", timeout: Math.max(1, timeout) });
    await viewport.evaluate((element) => {
      element.scrollTop = 0;
      element.dispatchEvent(new Event("scroll", { bubbles: true }));
    });

    await expect
      .poll(
        async () => {
          if (await node.isVisible().catch(() => false)) return true;
          await viewport
            .evaluate((element) => {
              const maxScrollTop = Math.max(0, element.scrollHeight - element.clientHeight);
              const increment = Math.max(element.clientHeight * 0.8, 160);
              const nextScrollTop = Math.min(maxScrollTop, element.scrollTop + increment);
              if (nextScrollTop <= element.scrollTop + 1) return false;
              element.scrollTop = nextScrollTop;
              element.dispatchEvent(new Event("scroll", { bubbles: true }));
              return true;
            })
            .catch(() => false);
          return false;
        },
        {
          timeout,
          intervals: [100, 250, 500],
          message: `Waiting for file tree node ${nodePath} to mount`,
        },
      )
      .toBe(true);
    return node;
  }

  /** The existing Files viewport that owns tree scrolling. */
  fileTreeScrollViewport(): Locator {
    return this.page.locator('[data-testid="file-tree-scroll"]:visible');
  }

  /** Visible tree rows, including only rows currently mounted by the tree. */
  visibleFileTreeNodes(): Locator {
    return this.page.locator('[data-testid="file-tree-node"]:visible');
  }

  /** Visible search button in the Files panel, including the mobile panel mount. */
  fileSearchButton(): Locator {
    return this.page.locator('button[aria-label="Search files"]:visible');
  }

  /** Search input shown in the visible Files panel. */
  fileSearchInput(): Locator {
    return this.page.locator('input[placeholder="Search files..."]:visible');
  }

  /** Search result by its task-root-relative path. */
  fileSearchResult(nodePath: string): Locator {
    return this.page.locator(
      `[data-testid="file-search-result"][data-path=${JSON.stringify(nodePath)}]:visible`,
    );
  }

  /** All file tree nodes with data-selected="true". */
  fileTreeSelectedNodes(): Locator {
    return this.files.locator("[data-selected='true']");
  }

  /** The desktop context-menu action for a selected file-tree node. */
  fileTreeAddToChatContextMenuItem(): Locator {
    return this.page.locator(
      '[data-slot="context-menu-content"][data-state="open"] [data-testid="file-context-add-to-chat"]',
    );
  }

  /** Visible coarse-pointer row action for one file-tree node. */
  fileTreeNodeActions(nodePath: string): Locator {
    return this.page.locator(
      `[data-testid="file-tree-node-actions"][data-path=${JSON.stringify(nodePath)}]:visible`,
    );
  }

  /** Responsive dropdown opened from a file-tree row action. */
  fileTreeTouchMenu(): Locator {
    return this.page.locator(
      '[data-slot="dropdown-menu-content"][data-state="open"][data-testid="file-tree-touch-menu"]',
    );
  }

  /** Add-to-chat item inside the responsive file-tree dropdown. */
  fileTreeTouchAddToChatContextItem(): Locator {
    return this.page.locator(
      '[data-slot="dropdown-menu-content"][data-state="open"] [data-testid="file-tree-touch-add-to-chat"]',
    );
  }

  /** Pending composer chip for a file or directory path. */
  chatContextFile(path: string): Locator {
    return this.activeChat().locator(
      `[data-testid="chat-context-file"][data-path=${JSON.stringify(path)}]`,
    );
  }

  /** Context-file badge on a sent user message. */
  sentMessageContextFile(path: string): Locator {
    return this.activeChat().locator(
      `[data-testid="message-context-file"][data-path=${JSON.stringify(path)}]`,
    );
  }
}
