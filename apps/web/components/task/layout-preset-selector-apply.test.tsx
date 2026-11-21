import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { getEnvLayout } from "@/lib/local-storage";
import { getBuiltInLayoutProfile } from "@/lib/layout/layout-profiles";
import {
  mountDockview,
  disposeDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
import { LayoutPresetSelector } from "./layout-preset-selector";
import { createAppStore } from "@/lib/state/store";

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
it.each(["current", "environment", "selection-aba", "newer-layout"])(
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
      created_at: "2026-11-21T00:00:00Z",
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
      if (winner === "current")
        expect(useDockviewStore.getState().activeLayoutProfile).toEqual({
          kind: "custom",
          id: "saved-0",
        });
      else {
        expect(api.toJSON()).toEqual(before);
        expect(useDockviewStore.getState().activeLayoutProfile).toEqual(profile);
        expect(getEnvLayout(useDockviewStore.getState().currentLayoutEnvId!)).toEqual(persisted);
      }
    } finally {
      cleanup();
      disposeDockview(api);
      useDockviewStore.setState(original);
    }
  },
);
