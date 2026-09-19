import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SessionMobileTopBar } from "./session-mobile-top-bar";

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => null,
}));
vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));
vi.mock("@/components/gitlab/mr-topbar-button", () => ({ MRTopbarButton: () => null }));
vi.mock("@/components/task/port-forward-dialog", () => ({ PortForwardButton: () => null }));
vi.mock("@/components/task/task-top-bar-plugin-actions", () => ({
  TaskTopBarPluginActions: () => null,
}));
vi.mock("@/components/navigation/app-nav-sheet", () => ({
  AppNavSheet: () => <button aria-label="Open navigation menu" />,
}));
afterEach(cleanup);

describe("phone task navigation controls", () => {
  // @covers AC-UI-MOBILE-MENU-002.1, AC-UI-MOBILE-MENU-002.3
  it("opens task switching from the title and exposes its expanded state", () => {
    const onTaskPickerClick = vi.fn();
    const host = render(
      <SessionMobileTopBar
        taskTitle="Fix checkout"
        onTaskPickerClick={onTaskPickerClick}
        taskPickerOpen={false}
      />,
    );
    const title = screen.getByRole("button", { name: /Fix checkout/ });
    fireEvent.click(title);
    expect(onTaskPickerClick).toHaveBeenCalledOnce();
    expect(title.getAttribute("aria-expanded")).toBe("false");
    host.rerender(
      <SessionMobileTopBar
        taskTitle="Fix checkout"
        onTaskPickerClick={onTaskPickerClick}
        taskPickerOpen
      />,
    );
    expect(title.getAttribute("aria-expanded")).toBe("true");
  });

  it("keeps app navigation separate from task selection", () => {
    const onTaskPickerClick = vi.fn();
    render(
      <SessionMobileTopBar
        taskTitle="Fix checkout"
        onTaskPickerClick={onTaskPickerClick}
        taskPickerOpen={false}
      />,
    );
    fireEvent.click(screen.getByRole("button", { name: "Open navigation menu" }));
    expect(onTaskPickerClick).not.toHaveBeenCalled();
  });
});
