import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getEnvLayout, getEnvMaximizeState } from "@/lib/local-storage";
import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";
import {
  mountDockview,
  disposeDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
import { LayoutPresetSelector } from "./layout-preset-selector";
import { createAppStore } from "@/lib/state/store";
import { setupPortalCleanup } from "./dockview-layout-setup";

const CREATED_AT = "2026-11-21T00:00:00Z";
const load = vi.hoisted(() => vi.fn());
vi.mock("@/hooks/use-task-sessions", () => ({
  useTaskSessions: () => ({ isLoaded: false, loadSessions: load }),
}));
let store: ReturnType<typeof useAppStoreApi>;
function Probe() {
  store = useAppStoreApi();
  return <LayoutPresetSelector />;
}
async function choose(name: string) {
  const trigger = screen.getByTestId("layout-preset-trigger");
  fireEvent.pointerDown(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.pointerUp(trigger, { button: 0, ctrlKey: false, pointerType: "mouse" });
  fireEvent.click(trigger);
  fireEvent.click(await screen.findByRole("menuitem", { name }));
}
const settle = () => new Promise<void>((resolve) => requestAnimationFrame(() => resolve()));
afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
  sessionStorage.clear();
});
const SAME_ENV_MAX = "same-env-max";
it.each(["current", "environment", "selection-aba", "newer-layout", SAME_ENV_MAX])(
  "admits the real Saved Layout click only under its original owner: %s",
  async (winner) => {
    const api = mountDockview(1200, 700);
    const original = useDockviewStore.getState();
    const held = Promise.withResolvers<void>();
    load.mockReset().mockReturnValueOnce(held.promise).mockResolvedValue(undefined);
    useDockviewStore.setState({
      api,
      currentLayoutEnvId: "click-x",
      isRestoringLayout: false,
      preMaximizeLayout: null,
    });
    useDockviewStore.getState().applyBuiltInPreset("default");
    await act(settle);
    const layouts = ["Held preset", "Winning preset"].map((name, index) => ({
      id: `saved-${index}`,
      name,
      is_default: false,
      created_at: CREATED_AT,
      layout: getBuiltInLayoutProfile(index ? "plan" : "preview").layout,
    }));
    const base = createAppStore().getState();
    render(
      <StateProvider
        initialState={{
          tasks: { ...base.tasks, activeTaskId: "t1", activeSessionId: "s1" },
          userSettings: { ...base.userSettings, savedLayouts: layouts },
        }}
      >
        <TooltipProvider>
          <Probe />
        </TooltipProvider>
      </StateProvider>,
    );
    try {
      if (winner === SAME_ENV_MAX) {
        useDockviewStore.getState().maximizeGroup(api.getPanel("chat")!.group.id);
        await act(settle);
      }
      await choose("Held preset");
      await waitFor(() => expect(load).toHaveBeenCalledTimes(1));
      if (winner === "environment")
        await act(async () => {
          store.getState().setActiveSession("t2", "s2");
          useDockviewStore.getState().switchEnvLayout("click-x", "click-y", null, []);
          await settle();
        });
      if (winner === "selection-aba")
        act(() => {
          store.getState().setActiveSession("t1", "s2");
          store.getState().setActiveSession("t1", "s1");
        });
      if (winner === "newer-layout") {
        await choose("Winning preset");
        await act(settle);
      }
      const before = api.toJSON();
      const profile = useDockviewStore.getState().activeLayoutProfile;
      const persisted = getEnvLayout(useDockviewStore.getState().currentLayoutEnvId!);
      await act(async () => {
        held.resolve();
        await settle();
      });
      if (winner === "current" || winner === SAME_ENV_MAX)
        expect(useDockviewStore.getState().activeLayoutProfile).toEqual({
          kind: "custom",
          id: "saved-0",
        });
      else {
        expect(api.toJSON()).toEqual(before);
        expect(useDockviewStore.getState().activeLayoutProfile).toEqual(profile);
        expect(getEnvLayout(useDockviewStore.getState().currentLayoutEnvId!)).toEqual(persisted);
      }
      if (winner === SAME_ENV_MAX) {
        expect(useDockviewStore.getState().preMaximizeLayout).toBeNull();
        expect(useDockviewStore.getState().maximizedGroupId).toBeNull();
        expect(getEnvMaximizeState("click-x")).toBeNull();
        const replacement = api.toJSON();
        useDockviewStore.getState().exitMaximizedLayout();
        expect(api.toJSON()).toEqual(replacement);
        expect(getEnvLayout("click-x")).not.toBeNull();
      }
    } finally {
      cleanup();
      disposeDockview(api);
      useDockviewStore.setState(original);
    }
  },
);

it.each(
  ["legacy", "columns"].flatMap((shape) =>
    ["entry", "restored", "settled"].map((mode) => ({ shape, mode })),
  ),
)(
  "preserves the $mode maximize owner when a $shape Saved Layout fails",
  async ({ mode, shape }) => {
    const api = mountDockview(1200, 700);
    const original = useDockviewStore.getState();
    try {
      useDockviewStore.setState({ api, currentLayoutEnvId: "failed-x", preMaximizeLayout: null });
      useDockviewStore.getState().applyBuiltInPreset("default");
      await act(settle);
      if (mode !== "entry") {
        useDockviewStore.getState().maximizeGroup(api.getPanel("chat")!.group.id);
        await act(settle);
        if (mode === "restored") {
          useDockviewStore.getState().switchEnvLayout("failed-x", "failed-y", null, []);
          await act(settle);
        }
      }
      const frames: FrameRequestCallback[] = [];
      vi.spyOn(window, "requestAnimationFrame").mockImplementation((frame) => {
        frames.push(frame);
        return frames.length;
      });
      if (mode === "entry")
        useDockviewStore.getState().maximizeGroup(api.getPanel("chat")!.group.id);
      else if (mode === "restored")
        useDockviewStore.getState().switchEnvLayout("failed-y", "failed-x", null, []);
      api.getPanel("chat")!.api.setActive();
      const overlay = api.toJSON();
      const parked = useDockviewStore.getState().preMaximizeLayout;
      const profile = useDockviewStore.getState().activeLayoutProfile;
      const saved = getEnvMaximizeState("failed-x");
      vi.spyOn(api, "fromJSON").mockImplementationOnce(() => {
        throw new Error("native restore failed");
      });
      useDockviewStore.getState().applyCustomLayout({
        id: "failed",
        name: "Failed preset",
        isDefault: false,
        createdAt: CREATED_AT,
        layout:
          shape === "legacy"
            ? JSON.parse(JSON.stringify(overlay))
            : getBuiltInLayoutProfile("default")!.layout,
      });
      expect(api.toJSON()).toEqual(overlay);
      expect(useDockviewStore.getState().preMaximizeLayout).toBe(parked);
      expect(useDockviewStore.getState().activeLayoutProfile).toBe(profile);
      expect(getEnvMaximizeState("failed-x")).toEqual(saved);
      act(() => {
        for (const frame of [...frames]) frame(0);
      });
      expect(useDockviewStore.getState().isRestoringLayout).toBe(false);
    } finally {
      disposeDockview(api);
      useDockviewStore.setState(original);
    }
  },
);

it.each([false, true])(
  "rejects an invalid legacy preset without detaching transcript ownership (overlay=%s)",
  async (overlay) => {
    const api = mountDockview(1200, 700);
    const previous = useDockviewStore.getState();
    const app = createAppStore();
    try {
      useDockviewStore.setState({ api, currentLayoutEnvId: "invalid", preMaximizeLayout: null });
      useDockviewStore.getState().applyBuiltInPreset("default");
      await act(settle);
      if (overlay) {
        useDockviewStore.getState().maximizeGroup(api.getPanel("chat")!.group.id);
        await act(settle);
      }
      app.setState({ tasks: { ...app.getState().tasks, activeSessionId: "B" } });
      setupPortalCleanup(api, app);
      const target = { sessionId: "B", messageId: "indexed", token: 100, hostPanelId: "chat" };
      useDockviewStore.setState({ scrollTarget: target });
      const panel = api.getPanel("chat");
      const fromJSON = vi.spyOn(api, "fromJSON");
      const profile = useDockviewStore.getState().activeLayoutProfile;
      useDockviewStore.getState().applyCustomLayout({
        id: "invalid",
        name: "Invalid",
        isDefault: false,
        createdAt: CREATED_AT,
        layout: {},
      });
      expect(useDockviewStore.getState().scrollTarget).toEqual(target);
      expect(api.getPanel("chat")).toBe(panel);
      expect(fromJSON).not.toHaveBeenCalled();
      expect(useDockviewStore.getState().activeLayoutProfile).toBe(profile);
    } finally {
      disposeDockview(api);
      useDockviewStore.setState(previous);
    }
  },
);

it.each(["legacy", "columns"])(
  "settles metadata after a failed %s native rollback without retrying it",
  async (shape) => {
    const api = mountDockview(1200, 700);
    const previous = useDockviewStore.getState();
    try {
      useDockviewStore.setState({ api, currentLayoutEnvId: "rollback", preMaximizeLayout: null });
      useDockviewStore.getState().applyBuiltInPreset("default");
      await act(settle);
      const grid = api.toJSON();
      const profile = useDockviewStore.getState().activeLayoutProfile;
      const fromJSON = vi.spyOn(api, "fromJSON").mockImplementation(() => {
        throw new Error("native and rollback failure");
      });
      useDockviewStore.getState().applyCustomLayout({
        id: "failure",
        name: "Failure",
        isDefault: false,
        createdAt: CREATED_AT,
        layout:
          shape === "legacy"
            ? JSON.parse(JSON.stringify(grid))
            : getBuiltInLayoutProfile("default")!.layout,
      });
      expect(fromJSON).toHaveBeenCalledTimes(2);
      expect(useDockviewStore.getState().isRestoringLayout).toBe(false);
      expect(useDockviewStore.getState().activeLayoutProfile).toBe(profile);
    } finally {
      vi.restoreAllMocks();
      disposeDockview(api);
      useDockviewStore.setState(previous);
    }
  },
);
