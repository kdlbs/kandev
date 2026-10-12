import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

const IDS = vi.hoisted(() => ({
  launcher: "workspace-copilot-launcher",
  panel: "stub-panel",
  main: "page-main",
  goneButton: "stub-gone",
  switchButton: "stub-switch",
  lastUsedKey: "kandev.coordinatorCopilot.lastUsed.ws-1",
}));

const mocks = vi.hoisted(() => ({
  flags: { coordinator: true, coordinatorPhase2: true },
  scope: {
    workspace: { scopes: ["workspace.manage"] } as { scopes: string[] } | undefined,
    workspaceId: "ws-1" as string | null,
    mode: "kanban" as "kanban" | "office" | "unknown",
  },
  route: { kind: "kanban" } as { kind: string; taskId?: string },
  listCoordinators: vi.fn(),
  panelRenders: [] as string[],
  taskHasWalkthrough: false,
}));

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: (name: "coordinator" | "coordinatorPhase2") => mocks.flags[name],
}));
vi.mock("@/components/workspace-scope-provider", () => ({
  useWorkspaceScope: () => mocks.scope,
}));
vi.mock("@/src/spa-routes", () => ({ useResolvedSpaRoute: () => mocks.route }));
vi.mock("@/lib/api/domains/coordinator-api", () => ({
  listCoordinators: mocks.listCoordinators,
  getCoordinator: vi.fn(),
}));
vi.mock("@/components/right-side-panel", () => ({
  RightSidePanel: ({
    open,
    main,
    children,
  }: {
    open: boolean;
    main: ReactNode;
    children: ReactNode;
  }) => (
    <div>
      <div data-testid={IDS.main}>{main}</div>
      {open && <div data-testid="rsp-panel">{children}</div>}
    </div>
  ),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (key: string) => key }) }));
vi.mock("./use-task-has-walkthrough", () => ({
  useTaskHasWalkthrough: () => mocks.taskHasWalkthrough,
}));
vi.mock("./use-page-context", async (importOriginal) => ({
  ...(await importOriginal<typeof import("./use-page-context")>()),
  useActiveWorkflowId: () => null,
  usePageContext: () => null,
}));
vi.mock("./workspace-copilot-panel", () => ({
  WorkspaceCopilotPanel: (props: {
    workspaceId: string;
    coordinator: Coordinator;
    onGone: (id: string) => void;
    onSwitch: (id: string) => void;
  }) => {
    mocks.panelRenders.push(`${props.workspaceId}/${props.coordinator.id}`);
    return (
      <div data-testid={IDS.panel} data-coordinator={props.coordinator.id}>
        <button
          type="button"
          data-testid={IDS.goneButton}
          onClick={() => props.onGone(props.coordinator.id)}
        />
        <button
          type="button"
          data-testid={IDS.switchButton}
          onClick={() => props.onSwitch("c-2")}
        />
      </div>
    );
  },
}));

import { WorkspaceCopilotHost } from "./workspace-copilot-host";

const coordinator = (id: string) => ({ id, name: id }) as Coordinator;
const LIST = { coordinators: [coordinator("c-1"), coordinator("c-2")] };

function tree() {
  return (
    <TooltipProvider>
      <WorkspaceCopilotHost>
        <span>page</span>
      </WorkspaceCopilotHost>
    </TooltipProvider>
  );
}

function mount() {
  return render(tree());
}

beforeEach(() => {
  window.localStorage.clear();
  mocks.taskHasWalkthrough = false;
  mocks.flags.coordinator = true;
  mocks.flags.coordinatorPhase2 = true;
  mocks.scope.workspace = { scopes: ["workspace.manage"] };
  mocks.scope.workspaceId = "ws-1";
  mocks.scope.mode = "kanban";
  mocks.route = { kind: "kanban" };
  mocks.panelRenders.length = 0;
  mocks.listCoordinators.mockReset();
  mocks.listCoordinators.mockResolvedValue(LIST);
});
afterEach(() => cleanup());

const launcher = () => screen.queryByTestId(IDS.launcher);
const panelCoordinator = () => screen.queryByTestId(IDS.panel)?.getAttribute("data-coordinator");

describe("WorkspaceCopilotHost eligibility", () => {
  it.each([
    ["the board", { kind: "kanban" }],
    ["a task page", { kind: "taskDetail", taskId: "t-1" }],
    ["the Inbox", { kind: "needsYouInbox" }],
  ])("offers the launcher on %s", async (_name, route) => {
    mocks.route = route;
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
  });

  it("keeps the launcher in the corner on a task page without a walkthrough", async () => {
    mocks.route = { kind: "taskDetail", taskId: "t-1" };
    mocks.taskHasWalkthrough = false;
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    expect(launcher()?.className).toContain("bottom-[calc(1.5rem");
    expect(launcher()?.className).not.toContain("bottom-[calc(5.5rem");
  });

  it("lifts the launcher above the walkthrough launcher when the task has a walkthrough", async () => {
    mocks.route = { kind: "taskDetail", taskId: "t-1" };
    mocks.taskHasWalkthrough = true;
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    expect(launcher()?.className).toContain("bottom-[calc(5.5rem");
  });

  it.each([
    ["settings", { kind: "settings" }],
    ["the coordinator screens", { kind: "coordinator" }],
    ["office", { kind: "office" }],
  ])("offers nothing on %s and requests nothing", async (_name, route) => {
    mocks.route = route;
    mount();
    await act(async () => {});
    expect(launcher()).toBeNull();
    expect(mocks.listCoordinators).not.toHaveBeenCalled();
  });

  it("offers nothing without workspace.manage, in an Office workspace or with the flag off", async () => {
    mocks.scope.workspace = { scopes: [] };
    const first = mount();
    await act(async () => {});
    expect(launcher()).toBeNull();
    first.unmount();

    mocks.scope.workspace = { scopes: ["workspace.manage"] };
    mocks.scope.mode = "office";
    const second = mount();
    await act(async () => {});
    expect(launcher()).toBeNull();
    second.unmount();

    mocks.scope.mode = "kanban";
    mocks.flags.coordinatorPhase2 = false;
    mount();
    await act(async () => {});
    expect(launcher()).toBeNull();
    expect(mocks.listCoordinators).not.toHaveBeenCalled();
    expect(screen.getByText("page")).toBeTruthy();
  });

  it("offers nothing while the list is empty or unloaded", async () => {
    mocks.listCoordinators.mockResolvedValue({ coordinators: [] });
    mount();
    await waitFor(() => expect(mocks.listCoordinators).toHaveBeenCalled());
    await act(async () => {});
    expect(launcher()).toBeNull();
  });

  it("keeps the launcher when the route changes during the first list read", async () => {
    let resolveList: (value: unknown) => void = () => {};
    mocks.listCoordinators.mockReturnValue(new Promise((resolve) => (resolveList = resolve)));
    const view = mount();
    mocks.route = { kind: "taskDetail", taskId: "t-1" };
    view.rerender(tree());
    await act(async () => resolveList(LIST));
    expect(launcher()).not.toBeNull();
    expect(mocks.listCoordinators).toHaveBeenCalledTimes(1);
  });
});

describe("WorkspaceCopilotHost opening and choosing", () => {
  it("opens with the first coordinator and re-reads the list", async () => {
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    expect(panelCoordinator()).toBe("c-1");
    expect(launcher()).toBeNull();
    await waitFor(() => expect(mocks.listCoordinators).toHaveBeenCalledTimes(2));
  });

  it("opens with the last used coordinator and forgets a stale one", async () => {
    window.localStorage.setItem(IDS.lastUsedKey, "c-2");
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    expect(panelCoordinator()).toBe("c-2");

    cleanup();
    window.localStorage.setItem(IDS.lastUsedKey, "gone");
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    expect(panelCoordinator()).toBe("c-1");
    expect(window.localStorage.getItem(IDS.lastUsedKey)).toBeNull();
  });

  it("a switch writes last used and gives the new coordinator the panel", async () => {
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    fireEvent.click(screen.getByTestId(IDS.switchButton));
    expect(panelCoordinator()).toBe("c-2");
    expect(window.localStorage.getItem(IDS.lastUsedKey)).toBe("c-2");
  });
});

describe("WorkspaceCopilotHost losing the coordinator or eligibility", () => {
  it("moves to the first remaining coordinator when the open one is gone", async () => {
    mocks.listCoordinators.mockResolvedValue({
      coordinators: [coordinator("c-1"), coordinator("c-2")],
    });
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    mocks.listCoordinators.mockResolvedValue({ coordinators: [coordinator("c-2")] });
    fireEvent.click(screen.getByTestId(IDS.goneButton));
    await waitFor(() => expect(panelCoordinator()).toBe("c-2"));
  });

  it("closes and drops the launcher when no coordinator remains", async () => {
    mocks.listCoordinators.mockResolvedValue({ coordinators: [coordinator("c-1")] });
    mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    mocks.listCoordinators.mockResolvedValue({ coordinators: [] });
    fireEvent.click(screen.getByTestId(IDS.goneButton));
    await waitFor(() => expect(panelCoordinator()).toBeUndefined());
    expect(launcher()).toBeNull();
  });

  it("closes when the route leaves the workspace pages and stays closed on return", async () => {
    const view = mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    const rerender = () => view.rerender(tree());
    mocks.route = { kind: "settings" };
    rerender();
    await waitFor(() => expect(panelCoordinator()).toBeUndefined());
    mocks.route = { kind: "kanban" };
    rerender();
    await waitFor(() => expect(launcher()).not.toBeNull());
    expect(panelCoordinator()).toBeUndefined();
  });

  it("closes on a workspace change", async () => {
    const view = mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    mocks.scope.workspaceId = "ws-2";
    view.rerender(tree());
    await waitFor(() => expect(panelCoordinator()).toBeUndefined());
  });

  it("never renders a coordinator against another workspace on a workspace change", async () => {
    const view = mount();
    await waitFor(() => expect(launcher()).not.toBeNull());
    fireEvent.click(launcher()!);
    await waitFor(() => expect(panelCoordinator()).toBe("c-1"));
    mocks.panelRenders.length = 0;
    mocks.listCoordinators.mockResolvedValue({ coordinators: [coordinator("c-9")] });
    mocks.scope.workspaceId = "ws-2";
    view.rerender(tree());
    await waitFor(() => expect(launcher()).not.toBeNull());
    expect(mocks.panelRenders).toEqual([]);
  });
});
