import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { IDockviewPanelHeaderProps } from "dockview-react";

const state = vi.hoisted(() => ({
  activeSessionId: "session",
  gitStatus: undefined as unknown,
  commitsLoaded: false,
  totalCount: 0,
  isActive: false,
  activeChangeListeners: [] as Array<(event: { isActive: boolean }) => void>,
  groupElement: null as HTMLElement | null,
  activate: vi.fn(),
}));

vi.mock("dockview-react", () => ({
  DockviewDefaultTab: () => <div>Changes</div>,
}));

vi.mock("@kandev/ui/context-menu", () => {
  const Wrapper = ({ children, ...props }: React.ComponentProps<"div">) => (
    <div {...props}>{children}</div>
  );
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
  get isActive() {
    return state.isActive;
  },
  group: {
    get element() {
      return state.groupElement!;
    },
    panels: [],
  },
  onDidActiveChange: (listener: (event: { isActive: boolean }) => void) => {
    state.activeChangeListeners.push(listener);
    return { dispose: vi.fn() };
  },
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
  state.isActive = false;
  state.activeChangeListeners = [];
  state.groupElement = document.createElement("div");
  document.body.appendChild(state.groupElement);
  state.activate.mockClear();
});

afterEach(() => {
  cleanup();
  state.groupElement?.remove();
});

function changesTabTree() {
  return (
    <div className="dv-tab">
      <ChangesTab {...props} />
    </div>
  );
}

function setChangesTabActive(isActive: boolean) {
  state.isActive = isActive;
  for (const listener of state.activeChangeListeners) listener({ isActive });
}

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

  it("keeps Files selected after a user switches away before a delayed git update", () => {
    const view = render(changesTabTree());
    state.groupElement!.appendChild(view.container);
    const filesTab = document.createElement("div");
    filesTab.className = "dv-tab";
    state.groupElement!.appendChild(filesTab);

    state.gitStatus = {};
    state.commitsLoaded = true;
    view.rerender(changesTabTree());

    fireEvent.pointerDown(screen.getByTestId("dockview-tab-changes"));
    setChangesTabActive(true);
    fireEvent.pointerDown(filesTab);
    setChangesTabActive(false);

    state.totalCount = 1;
    view.rerender(changesTabTree());
    expect(state.activate).not.toHaveBeenCalled();

    fireEvent.pointerDown(screen.getByTestId("dockview-tab-changes"));
    setChangesTabActive(true);
    state.totalCount = 2;
    view.rerender(changesTabTree());
    expect(state.activate).toHaveBeenCalledOnce();
  });
});
