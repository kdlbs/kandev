import { describe, expect, it, vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import type { Message } from "@/lib/types/http";
import {
  resolveConditionalTodoPanelAction,
  resolveConfiguredTodoPanelPlacement,
  syncConditionalTodoPanel,
  todoListNotEmptyForSession,
  type SyncConditionalTodoPanelOptions,
} from "./dockview-todo-panel-sync";

const CENTER_GROUP_ID = "group-center";
const RIGHT_TOP_GROUP_ID = "group-right-top";

const DEFAULT_OPTIONS = {
  centerGroupId: CENTER_GROUP_ID,
  isRestoringLayout: false,
  isMaximized: false,
  onlyPinWhenNotEmpty: false,
  todoListNotEmpty: false,
  hasActiveSession: true,
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

type SyncOverrides = Pick<SyncConditionalTodoPanelOptions, "showTodoListPanel" | "settingsLoaded"> &
  Partial<Omit<SyncConditionalTodoPanelOptions, "showTodoListPanel" | "settingsLoaded">>;

/** Runs the sync with DEFAULT_OPTIONS merged over `overrides` — every
 *  syncConditionalTodoPanel test shares this exact options base. */
function runSync(api: DockviewApi, overrides: SyncOverrides): boolean {
  return syncConditionalTodoPanel(api, { ...DEFAULT_OPTIONS, ...overrides });
}

/** Asserts the sync resolves to "none": neither addPanel nor close called. */
function expectNoAction(overrides: SyncOverrides, panel?: { groupId?: string }): void {
  const { api, addPanel, close } = makeApi(panel);
  expect(runSync(api, overrides)).toBe(false);
  expect(addPanel).not.toHaveBeenCalled();
  expect(close).not.toHaveBeenCalled();
}

type DecisionCaseInput = {
  showTodoListPanel: boolean;
  panelExists: boolean;
  onlyPinWhenNotEmpty?: boolean;
  todoListNotEmpty?: boolean;
  hasActiveSession?: boolean;
  settingsLoaded?: boolean;
  isRestoringLayout?: boolean;
  isMaximized?: boolean;
};

const CONDITIONAL_TODO_PANEL_CASES: Array<[string, DecisionCaseInput, "add" | "remove" | "none"]> =
  [
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
    [
      "does not pin when the sub-option is on and the todo list is empty",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: true,
        todoListNotEmpty: false,
        panelExists: false,
      },
      "none",
    ],
    [
      "pins when the sub-option is on and the todo list is not empty",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: true,
        todoListNotEmpty: true,
        panelExists: false,
      },
      "add",
    ],
    [
      "leaves an existing panel alone when the sub-option is on and the list is empty",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: true,
        todoListNotEmpty: false,
        panelExists: true,
      },
      "none",
    ],
    [
      "pins regardless of the list when the sub-option is off",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        panelExists: false,
      },
      "add",
    ],
    [
      "still removes when the master preference is off, even with the sub-option on",
      {
        showTodoListPanel: false,
        onlyPinWhenNotEmpty: true,
        todoListNotEmpty: false,
        panelExists: true,
      },
      "remove",
    ],
    [
      "never adds without an active session, even with the master preference on",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        hasActiveSession: false,
        panelExists: false,
      },
      "none",
    ],
    [
      "still removes when the master preference is off without an active session",
      {
        showTodoListPanel: false,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        hasActiveSession: false,
        panelExists: true,
      },
      "remove",
    ],
    [
      "leaves an existing panel alone for a sessionless task with the preference on",
      {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        hasActiveSession: false,
        panelExists: true,
      },
      "none",
    ],
  ];

describe("resolveConditionalTodoPanelAction", () => {
  it.each(CONDITIONAL_TODO_PANEL_CASES)("%s", (_name, input, expected) => {
    expect(
      resolveConditionalTodoPanelAction({
        settingsLoaded: true,
        isRestoringLayout: false,
        isMaximized: false,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        hasActiveSession: true,
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

    expect(runSync(api, { showTodoListPanel: true, settingsLoaded: true })).toBe(true);
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
      runSync(api, {
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

    expect(runSync(api, { showTodoListPanel: true, settingsLoaded: true })).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(
      expect.objectContaining({ position: { referenceGroup: CENTER_GROUP_ID } }),
    );
  });

  it("closes an existing panel unconditionally when the preference turns off, even mid-restore", () => {
    const { api, close } = makeApi({});

    expect(
      runSync(api, { showTodoListPanel: false, settingsLoaded: true, isRestoringLayout: true }),
    ).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });

  it("does not add while restoring or maximized", () => {
    expectNoAction({ showTodoListPanel: true, settingsLoaded: true, isRestoringLayout: true });
    expectNoAction({ showTodoListPanel: true, settingsLoaded: true, isMaximized: true });
  });

  it("does nothing before settings have hydrated", () => {
    expectNoAction({ showTodoListPanel: false, settingsLoaded: false }, {});
  });

  it("does not add an absent panel when the sub-option is on and the list is empty", () => {
    expectNoAction({
      showTodoListPanel: true,
      onlyPinWhenNotEmpty: true,
      todoListNotEmpty: false,
      settingsLoaded: true,
    });
  });

  it("adds an absent panel when the sub-option is on and the list is not empty", () => {
    const { api, addPanel } = makeApi();

    expect(
      runSync(api, {
        showTodoListPanel: true,
        onlyPinWhenNotEmpty: true,
        todoListNotEmpty: true,
        settingsLoaded: true,
      }),
    ).toBe(true);
    expect(addPanel).toHaveBeenCalledWith(expect.objectContaining({ id: "todos", inactive: true }));
  });

  it("does not add for a sessionless task even when the preference is on", () => {
    expectNoAction({
      showTodoListPanel: true,
      onlyPinWhenNotEmpty: false,
      todoListNotEmpty: false,
      hasActiveSession: false,
      settingsLoaded: true,
    });
  });

  it("still removes a materialized panel for a sessionless task when the preference is off", () => {
    const { api, close } = makeApi({});

    expect(
      runSync(api, {
        showTodoListPanel: false,
        onlyPinWhenNotEmpty: false,
        todoListNotEmpty: false,
        hasActiveSession: false,
        settingsLoaded: true,
      }),
    ).toBe(true);
    expect(close).toHaveBeenCalledOnce();
  });
});

describe("todoListNotEmptyForSession", () => {
  const persistedTodoMessage = {
    id: "m1",
    type: "todo",
    turn_id: "turn-1",
    metadata: { todos: [{ text: "Write tests", done: true }] },
  } as unknown as Message;

  it("is false when the session has neither live entries nor a persisted todo message", () => {
    expect(todoListNotEmptyForSession(undefined, [])).toBe(false);
  });

  it("is true when the live slice has entries, even with no persisted messages", () => {
    expect(
      todoListNotEmptyForSession([{ description: "Implement", status: "in_progress" }], []),
    ).toBe(true);
  });

  it("falls back to the latest persisted todo message when the live slice is absent", () => {
    expect(todoListNotEmptyForSession(undefined, [persistedTodoMessage])).toBe(true);
  });

  it("treats an empty live array like an absent live slice (persisted fallback wins)", () => {
    expect(todoListNotEmptyForSession([], [persistedTodoMessage])).toBe(true);
    expect(todoListNotEmptyForSession([], [])).toBe(false);
  });

  it("treats malformed persisted todo metadata as empty instead of throwing", () => {
    const malformed = [
      { id: "m1", type: "todo", turn_id: "t", metadata: { todos: { text: "x" } } },
      { id: "m2", type: "todo", turn_id: "t", metadata: { todos: [null, 42] } },
    ] as unknown as Message[];

    for (const message of malformed) {
      expect(() => todoListNotEmptyForSession(undefined, [message])).not.toThrow();
      expect(todoListNotEmptyForSession(undefined, [message])).toBe(false);
    }
  });
});
