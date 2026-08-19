import type { ReactNode } from "react";
import { fireEvent, render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("@kandev/ui/context-menu", () => ({
  ContextMenu: ({ children }: { children: ReactNode }) => <>{children}</>,
  ContextMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

import { SessionTabs, type SessionTab } from "@/components/session-tabs";

describe("TaskRightPanel terminal tabs", () => {
  it("exposes separate rename and terminate actions from an ordinary tab", () => {
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
});
