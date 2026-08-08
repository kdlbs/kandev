import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import {
  resolveConditionalTodoPanelAction,
  resolveConfiguredTodoPanelPlacement,
  syncConditionalTodoPanel,
  type ConditionalTodoPanelOptions,
} from "./dockview-todo-panel-sync";

const CENTER_GROUP_ID = "group-center";
const RIGHT_TOP_GROUP_ID = "group-right-top";

const DEFAULT_OPTIONS: ConditionalTodoPanelOptions = {
  centerGroupId: CENTER_GROUP_ID,
  isRestoringLayout: false,
  isMaximized: false,
};

function makeApi(
  panel?: { groupId?: string },
  extraGroupIds: string[] = [RIGHT_TOP_GROUP_ID],
): { api: DockviewApi; close: ReturnType<typeof vi.fn>; addPanel: ReturnType<typeof vi.fn> } {
  const close = vi.fn();
  const addPanel = vi.fn();
  const todosPanel = panel
    ? { id: "todos", group: { id: panel.groupId ?? RIGHT_TOP_GROUP_ID }, api: { close } }
    : undefined;
  return {
    api: {
      getPanel: (id: string) => (id === "todos" ? todosPanel : undefined),
      groups: [{ id: CENTER_GROUP_ID }, ...extraGroupIds.map((id) => ({ id }))],
      addPanel,
      removePanel: vi.fn(),
    } as unknown as DockviewApi,
    close,
    addPanel,
  };
}

describe("resolveConditionalTodoPanelAction", () => {
  it.each([
    ["adds when enabled and absent", { showTodoListPanel: true, panelExists: false }, "add"],
    ["does nothing when already present", { showTodoListPanel: true, panelExists: true }, "none"],
    [
      "waits while restoring",
      { showTodoListPanel: true, panelExists: false, isRestoringLayout: true },
      "none",
    ],
    [
      "waits while maximized",
      { showTodoListPanel: true, panelExists: false, isMaximized: true },
      "none",
    ],
    [
      "removes when disabled and present",
      { showTodoListPanel: false, panelExists: true },
      "remove",
    ],
    [
      "does nothing when disabled and absent",
      { showTodoListPanel: false, panelExists: false },
      "none",
    ],
    [
      "waits for settings hydration before adding",
      { showTodoListPanel: true, panelExists: false, settingsLoaded: false },
      "none",
    ],
    [
      "waits for settings hydration before removing",
      { showTodoListPanel: false, panelExists: true, settingsLoaded: false },
      "none",
    ],
  ])("%s", (_name, input, expected) => {
    expect(
      resolveConditionalTodoPanelAction({
        settingsLoaded: true,
        isRestoringLayout: false,
        isMaximized: false,
        ...input,
      }),
    ).toBe(expected);
  });
});

describe("resolveConfiguredTodoPanelPlacement", () => {
  it("returns the saved group and tab index", () => {
    expect(
      resolveConfiguredTodoPanelPlacement({
        columns: [
          {
            id: "right",
            groups: [
              {
                id: RIGHT_TOP_GROUP_ID,
                panels: [
                  { id: "files", component: "files", title: "Files" },
                  { id: "todos", component: "todos", title: "Todos" },
                ],
              },
            ],
          },
        ],
      }),
    ).toEqual({ groupId: RIGHT_TOP_GROUP_ID, index: 1 });
  });

  it("returns no placement for the built-in Default", () => {
    expect(resolveConfiguredTodoPanelPlacement(null)).toBeNull();
  });
});

describe("syncConditionalTodoPanel", () => {
  it("adds an inactive panel beside Files/Changes when enabled and absent", () => {
    const { api, addPanel } = makeApi();

    expect(
      syncConditionalTodoPanel(api, {
        ...DEFAULT_OPTIONS,
        showTodoListPanel: true,
        settingsLoaded: true,
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({
        id: "todos",
        component: "todos",
        inactive: true,
        position: { referenceGroup: RIGHT_TOP_GROUP_ID },
      }),
    );
  });

  it("uses the configured group when a custom Default places todos elsewhere", () => {
    const { api, addPanel } = makeApi(undefined, ["custom-group"]);

    expect(
      syncConditionalTodoPanel(api, {
        ...DEFAULT_OPTIONS,
        showTodoListPanel: true,
        settingsLoaded: true,
        configuredPlacement: { groupId: "custom-group", index: 2 },
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({ position: { referenceGroup: "custom-group", index: 2 } }),
    );
  });

  it("falls back to the center group when no right column exists (e.g. compact layout)", () => {
    const { api, addPanel } = makeApi(undefined, []);

    expect(
      syncConditionalTodoPanel(api, {
        ...DEFAULT_OPTIONS,
        showTodoListPanel: true,
        settingsLoaded: true,
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({ position: { referenceGroup: CENTER_GROUP_ID } }),
    );
  });

  it("closes an existing panel unconditionally when the preference turns off, even mid-restore", () => {
    const { api, close } = makeApi({});

    expect(
      syncConditionalTodoPanel(api, {
        ...DEFAULT_OPTIONS,
        showTodoListPanel: false,
        settingsLoaded: true,
        isRestoringLayout: true,
      }),
    ).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });

  it("does not add while restoring or maximized", () => {
    for (const options of [
      { isRestoringLayout: true, isMaximized: false },
      { isRestoringLayout: false, isMaximized: true },
    ]) {
      const { api, addPanel } = makeApi();
      expect(
        syncConditionalTodoPanel(api, {
          ...DEFAULT_OPTIONS,
          showTodoListPanel: true,
          settingsLoaded: true,
          ...options,
        }),
      ).toBe(false);
      expect(addPanel).not.toHaveBeenCalled();
    }
  });

  it("does nothing before settings have hydrated", () => {
    const { api, addPanel, close } = makeApi({});
    expect(
      syncConditionalTodoPanel(api, {
        ...DEFAULT_OPTIONS,
        showTodoListPanel: false,
        settingsLoaded: false,
      }),
    ).toBe(false);
    expect(addPanel).not.toHaveBeenCalled();
    expect(close).not.toHaveBeenCalled();
  });
});
