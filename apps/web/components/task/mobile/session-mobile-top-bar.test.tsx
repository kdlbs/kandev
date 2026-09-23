import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import type { ReactElement, ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { SessionMobileTopBar } from "./session-mobile-top-bar";

const mocks = vi.hoisted(() => ({ pluginActions: undefined as unknown }));

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => null,
}));
vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ commits: [] }),
}));
vi.mock("@/components/gitlab/mr-topbar-button", () => ({ MRTopbarButton: () => null }));
vi.mock("@/components/task/port-forward-dialog", () => ({ PortForwardButton: () => null }));
vi.mock("@/components/task/task-top-bar-plugin-actions", () => ({
  TaskTopBarPluginActions: ({ presentation }: { presentation?: string }) => (
    <div data-presentation={presentation} data-testid="task-top-bar-plugin-actions" />
  ),
  useHasTaskTopBarPluginActions: () => true,
}));
vi.mock("@/components/navigation/app-nav-sheet", () => ({
  AppNavSheet: ({ pluginActions }: { pluginActions?: ReactNode }) => {
    mocks.pluginActions = pluginActions;
    return <button aria-label="Open navigation menu" />;
  },
}));
afterEach(() => {
  cleanup();
  mocks.pluginActions = undefined;
});

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

  // @covers AC-UI-MOBILE-TASK-CHROME-001.7
  it("moves session plugin actions out of the fixed header and into mobile navigation", () => {
    render(
      <SessionMobileTopBar
        taskId="task-1"
        workspaceId="workspace-1"
        sessionId="session-1"
        taskTitle="Fix checkout"
        onTaskPickerClick={vi.fn()}
        taskPickerOpen={false}
      />,
    );

    expect(screen.queryByTestId("task-top-bar-plugin-actions")).toBeNull();
    const pluginActions = mocks.pluginActions as ReactElement<{ presentation?: string }>;
    expect(pluginActions.props.presentation).toBe("mobile");
  });
});
