import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";
import type { DockviewApi } from "dockview-react";
import { pluginRegistry } from "@/lib/plugins/registry";
import { REUSABLE_PANEL_IDS } from "@/lib/state/layout-manager";
import {
  disposeDockview,
  mountDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";
import { LayoutEditorActions } from "./layout-editor-toolbar";

const PLUGIN_ID = "kandev-plugin-prompt-history";
const PROMPT_KEY = "prompt-history";
const OTHER_PLUGIN_ID = "kandev-plugin-notes";
const PLUGIN_PANEL_ID = `plugin:${PLUGIN_ID}:${PROMPT_KEY}`;
const ADD_PANEL = "Add panel";
const PROMPT_HISTORY = "Prompt History";
const MENU_ITEM = "menuitem";

let api: DockviewApi | null = null;

afterEach(() => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  pluginRegistry.unregisterPlugin(OTHER_PLUGIN_ID);
  if (api) disposeDockview(api);
  api = null;
});

function registerPluginPanel(pluginId: string, id: string, title: string): void {
  pluginRegistry.forPlugin(pluginId).registerTaskPanel({ id, title, Component: () => null });
}

/** Every built-in reusable panel in one group, leaving only plugin panels to add. */
function editorWithAllBuiltIns(extraIds: string[] = []): DockviewApi {
  const dockview = mountDockview(1000, 600);
  for (const id of [...REUSABLE_PANEL_IDS, ...extraIds]) {
    dockview.addPanel({ id, component: id.startsWith("plugin:") ? "plugin-panel" : id, title: id });
  }
  api = dockview;
  return dockview;
}

function renderActions(
  dockview: DockviewApi,
  onCommand = vi.fn(),
  selectedPanelId: string | null = null,
) {
  render(
    <TooltipProvider>
      <LayoutEditorActions
        api={dockview}
        anchor={null}
        editable
        selectedPanelId={selectedPanelId}
        onCommand={onCommand}
      />
    </TooltipProvider>,
  );
  return {
    onCommand,
    trigger: () => screen.getByRole<HTMLButtonElement>("button", { name: ADD_PANEL }),
  };
}

function openMenu(trigger: HTMLElement): void {
  fireEvent.click(trigger);
}

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.1, AC-UI-TASK-LAYOUT-PROFILES-002.2 */
describe("Add panel menu with plugin task panels", () => {
  it("keeps offering the missing built-in panels", () => {
    const dockview = mountDockview(1000, 600);
    dockview.addPanel({ id: "chat", component: "chat", title: "Agent" });
    api = dockview;
    const { trigger } = renderActions(dockview);

    openMenu(trigger());

    const offered = screen.getAllByRole(MENU_ITEM).map((item) => item.textContent);
    expect(offered).toContain("Files");
    expect(offered).not.toContain("Agent");
  });

  it("stays unavailable until a plugin registers, then offers it without a remount", () => {
    const { trigger } = renderActions(editorWithAllBuiltIns());
    expect(trigger().disabled).toBe(true);

    act(() => registerPluginPanel(PLUGIN_ID, PROMPT_KEY, PROMPT_HISTORY));

    expect(trigger().disabled).toBe(false);
    openMenu(trigger());
    expect(screen.getByRole(MENU_ITEM, { name: PROMPT_HISTORY })).toBeTruthy();
  });

  it("drops an option when its registration is revoked while the menu is open", () => {
    registerPluginPanel(PLUGIN_ID, PROMPT_KEY, PROMPT_HISTORY);
    registerPluginPanel(OTHER_PLUGIN_ID, "notes", "Notes");
    const { trigger } = renderActions(editorWithAllBuiltIns());
    openMenu(trigger());
    expect(screen.getAllByRole(MENU_ITEM).map((item) => item.textContent)).toEqual([
      PROMPT_HISTORY,
      "Notes",
    ]);

    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

    expect(screen.getAllByRole(MENU_ITEM).map((item) => item.textContent)).toEqual(["Notes"]);
  });

  it("omits identities already placed in any split and disables when nothing is missing", () => {
    registerPluginPanel(PLUGIN_ID, PROMPT_KEY, PROMPT_HISTORY);
    registerPluginPanel(OTHER_PLUGIN_ID, "notes", "Notes");
    const dockview = editorWithAllBuiltIns([PLUGIN_PANEL_ID]);
    const { trigger } = renderActions(dockview);

    openMenu(trigger());

    expect(screen.getAllByRole(MENU_ITEM).map((item) => item.textContent)).toEqual(["Notes"]);
    cleanup();
    const full = renderActions(
      editorWithAllBuiltIns([PLUGIN_PANEL_ID, `plugin:${OTHER_PLUGIN_ID}:notes`]),
    );
    expect(full.trigger().disabled).toBe(true);
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.7 */
describe("selecting a plugin option", () => {
  it("adds one generic panel in the selected split and reports one command", () => {
    registerPluginPanel(PLUGIN_ID, PROMPT_KEY, PROMPT_HISTORY);
    const dockview = editorWithAllBuiltIns();
    dockview
      .getPanel("files")!
      .api.moveTo({ group: dockview.getPanel("chat")!.group, position: "right" });
    const selectedGroupId = dockview.getPanel("files")!.group.id;
    const { onCommand } = renderActions(dockview, vi.fn(), "files");

    openMenu(screen.getByRole("button", { name: ADD_PANEL }));
    fireEvent.click(screen.getByRole(MENU_ITEM, { name: PROMPT_HISTORY }));

    const added = dockview.panels.filter((item) => item.id === PLUGIN_PANEL_ID);
    expect(added).toHaveLength(1);
    expect(added[0].group.id).toBe(selectedGroupId);
    expect(added[0].view.contentComponent).toBe("plugin-panel");
    expect(onCommand).toHaveBeenCalledTimes(1);
  });

  it("does not add a panel when a touch tap only opens the menu", () => {
    registerPluginPanel(PLUGIN_ID, PROMPT_KEY, PROMPT_HISTORY);
    const dockview = editorWithAllBuiltIns();
    const { trigger, onCommand } = renderActions(dockview);

    fireEvent.pointerDown(trigger(), { pointerType: "touch", button: 0 });
    fireEvent.pointerUp(trigger(), { pointerType: "touch", button: 0 });
    fireEvent.click(trigger());

    const menu = screen.getByRole("menu");
    expect(within(menu).getByRole(MENU_ITEM, { name: PROMPT_HISTORY })).toBeTruthy();
    expect(dockview.getPanel(PLUGIN_PANEL_ID)).toBeUndefined();
    expect(onCommand).not.toHaveBeenCalled();
  });
});
