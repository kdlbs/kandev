import type { Locator, Page } from "@playwright/test";

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
