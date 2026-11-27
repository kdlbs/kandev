import { cleanup, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IDockviewPanelHeaderProps } from "dockview-react";

const state = vi.hoisted(() => ({
  activeSessionId: "session",
  gitStatus: undefined as unknown,
  commitsLoaded: false,
  totalCount: 0,
  activate: vi.fn(),
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => <div>Changes</div>,
}));

vi.mock("@kandev/ui/context-menu", () => {
  const Wrapper = ({ children }: React.PropsWithChildren) => <div>{children}</div>;
  return {
    ContextMenu: Wrapper,
    ContextMenuContent: Wrapper,
    ContextMenuItem: Wrapper,
    ContextMenuTrigger: Wrapper,
  };
});

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (value: { tasks: { activeSessionId: string } }) => unknown) =>
    selector({ tasks: { activeSessionId: state.activeSessionId } }),
}));

vi.mock("@/hooks/domains/session/use-session-git-status", () => ({
  useSessionGitStatus: () => state.gitStatus,
}));

vi.mock("@/hooks/domains/session/use-session-commits", () => ({
  useSessionCommits: () => ({ loaded: state.commitsLoaded }),
}));

vi.mock("@/hooks/domains/session/use-session-changes-count", () => ({
  useSessionChangesCount: () => state.totalCount,
}));

vi.mock("./changes-panel-focus", () => ({
  autoActivateChangesPanel: state.activate,
}));

vi.mock("./use-tab-maximize", () => ({
  useTabMaximizeOnDoubleClick: () => vi.fn(),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}));

import { ChangesTab } from "./changes-tab";

const api = {
  id: "changes",
  isActive: false,
  group: { panels: [] },
  onDidActiveChange: () => ({ dispose: vi.fn() }),
};
const props = {
  api,
  containerApi: { removePanel: vi.fn() },
} as unknown as IDockviewPanelHeaderProps;

beforeEach(() => {
  state.activeSessionId = "session";
  state.gitStatus = undefined;
  state.commitsLoaded = false;
  state.totalCount = 0;
  state.activate.mockClear();
});

afterEach(() => cleanup());

describe("Changes tab initial activity baseline", () => {
  it("waits for both git status and the initial commit snapshot before auto-activating", () => {
    const view = render(<ChangesTab {...props} />);

    state.gitStatus = {};
    view.rerender(<ChangesTab {...props} />);
    state.totalCount = 55;
    view.rerender(<ChangesTab {...props} />);
    expect(state.activate).not.toHaveBeenCalled();

    state.commitsLoaded = true;
    view.rerender(<ChangesTab {...props} />);
    state.totalCount = 56;
    view.rerender(<ChangesTab {...props} />);
    expect(state.activate).toHaveBeenCalledTimes(1);
  });
});
