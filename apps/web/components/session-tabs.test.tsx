import type { RefObject } from "react";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

// Deliberately NOT mocking @kandev/ui/context-menu: the real Radix
// ContextMenuTrigger only conditionally renders its portal-based Content
// while closed, so `renderContextMenu` output (never wrapped in
// ContextMenuContent by these tests) still renders directly as a Root child —
// and using the real component is what let the data-state regression below
// reproduce at all (see #data-state test).
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

  it("keeps the tab's own active/inactive data-state when it has a context menu and no custom content", () => {
    // Regression: ContextMenuTrigger asChild clones its own open/closed
    // data-state onto its single child via Slot. When that child is the bare
    // TabsTrigger (no tab.content wrapper div in between), the clone lands
    // directly on TabsTrigger's DOM node and overwrites its active/inactive
    // data-state, silently breaking data-[state=active] styling.
    const tabs: SessionTab[] = [
      {
        id: "session-1",
        label: "Session 1",
        testId: "session-tab-1",
        renderContextMenu: () => null,
      },
      { id: "session-2", label: "Session 2", testId: "session-tab-2" },
    ];

    render(<SessionTabs tabs={tabs} activeTab="session-1" onTabChange={vi.fn()} />);

    expect(screen.getByTestId("session-tab-1").getAttribute("data-state")).toBe("active");
    expect(screen.getByTestId("session-tab-2").getAttribute("data-state")).toBe("inactive");
  });
});
