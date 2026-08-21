import type { ReactNode, RefObject } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

import { SessionTabs, type SessionTab } from "@/components/session-tabs";

describe("SessionTabs terminal tab contract", () => {
  afterEach(cleanup);

  it("exposes separate context actions from an ordinary tab", () => {
    const onRename = vi.fn();
    const onTerminate = vi.fn();
    const tabs: SessionTab[] = [
      {
        id: "terminal-1",
        label: "Terminal",
        testId: "terminal-tab-terminal-1",
        closable: true,
        renderContextMenu: () => (
          <>
            <button type="button" onClick={onRename}>
              Rename
            </button>
            <button type="button" onClick={onTerminate}>
              Terminate
            </button>
          </>
        ),
      },
    ];

    render(<SessionTabs tabs={tabs} activeTab="terminal-1" onTabChange={vi.fn()} />);

    fireEvent.click(screen.getByRole("button", { name: "Rename" }));
    fireEvent.click(screen.getByRole("button", { name: "Terminate" }));

    expect(onRename).toHaveBeenCalledOnce();
    expect(onTerminate).toHaveBeenCalledOnce();
  });

  it("passes a populated close anchor to the context-menu renderer", () => {
    let closeAnchorRef: RefObject<HTMLElement | null> | undefined;
    const tabs: SessionTab[] = [
      {
        id: "terminal-1",
        label: "Terminal",
        testId: "terminal-tab-terminal-1",
        closeTestId: "terminal-tab-close-terminal-1",
        closable: true,
        onClose: vi.fn(),
        renderContextMenu: (ref) => {
          closeAnchorRef = ref;
          return null;
        },
      },
    ];

    render(<SessionTabs tabs={tabs} activeTab="terminal-1" onTabChange={vi.fn()} />);

    expect(closeAnchorRef?.current).toBe(screen.getByTestId("terminal-tab-close-terminal-1"));
  });

  it("renders custom content beside the tab trigger without replacing tab semantics", () => {
    const tabs: SessionTab[] = [
      {
        id: "terminal-1",
        label: "Terminal",
        icon: <span data-testid="default-icon" />,
        badge: "#1",
        testId: "terminal-tab-terminal-1",
        content: <input aria-label="Rename terminal" data-testid="custom-content" />,
      },
    ];

    render(<SessionTabs tabs={tabs} activeTab="terminal-1" onTabChange={vi.fn()} />);

    const customContent = screen.getByTestId("custom-content");
    expect(customContent.closest("button")).toBeNull();
    expect(screen.queryByTestId("default-icon")).toBeNull();
    expect(screen.queryByText("#1")).toBeNull();
    expect(screen.queryByText("Terminal", { selector: "span.truncate" })).toBeNull();
    expect(screen.getByRole("tab", { name: "Terminal" })).toBeTruthy();
  });
});
