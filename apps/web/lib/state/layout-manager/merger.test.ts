import { describe, it, expect } from "vitest";
import type { LayoutState } from "./types";
import { mergePanelsIntoPreset } from "./merger";

const SESSION_ABC = "session:abc";
const SESSION_DEF = "session:def";

function makeLayout(centerPanels: Array<{ id: string; component: string }>): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        groups: [{ panels: [{ id: "sidebar", component: "sidebar", title: "Sidebar" }] }],
      },
      {
        id: "center",
        groups: [
          {
            panels: centerPanels.map((p) => ({ ...p, title: p.id })),
          },
        ],
      },
    ],
  };
}

function makeLayoutWithSide(
  centerPanels: Array<{ id: string; component: string }>,
  sideColumnId: string,
  sidePanels: Array<{ id: string; component: string }>,
): LayoutState {
  return {
    columns: [
      {
        id: "sidebar",
        groups: [{ panels: [{ id: "sidebar", component: "sidebar", title: "Sidebar" }] }],
      },
      {
        id: "center",
        groups: [{ panels: centerPanels.map((p) => ({ ...p, title: p.id })) }],
      },
      {
        id: sideColumnId,
        groups: [{ panels: sidePanels.map((p) => ({ ...p, title: p.id })) }],
      },
    ],
  };
}

function panelIdsIn(state: LayoutState, columnId: string): string[] {
  const col = state.columns.find((c) => c.id === columnId);
  return col?.groups.flatMap((g) => g.panels.map((p) => p.id)) ?? [];
}

describe("mergePanelsIntoPreset", () => {
  it("replaces chat panel with session panels when session panels exist", () => {
    const currentState = makeLayout([{ id: "session:abc123", component: "chat" }]);
    const targetPreset = makeLayout([{ id: "chat", component: "chat" }]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    const centerPanels = result.columns.find((c) => c.id === "center")!.groups[0].panels;
    const panelIds = centerPanels.map((p) => p.id);

    expect(panelIds).toContain("session:abc123");
    expect(panelIds).not.toContain("chat");
  });

  it("preserves chat panel when no session panels exist", () => {
    const currentState = makeLayout([{ id: "chat", component: "chat" }]);
    const targetPreset = makeLayout([{ id: "chat", component: "chat" }]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    const centerPanels = result.columns.find((c) => c.id === "center")!.groups[0].panels;
    const panelIds = centerPanels.map((p) => p.id);

    expect(panelIds).toContain("chat");
  });

  it("preserves multiple session panels and drops chat", () => {
    const currentState = makeLayout([
      { id: SESSION_ABC, component: "chat" },
      { id: SESSION_DEF, component: "chat" },
    ]);
    const targetPreset = makeLayout([{ id: "chat", component: "chat" }]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    const centerPanels = result.columns.find((c) => c.id === "center")!.groups[0].panels;
    const panelIds = centerPanels.map((p) => p.id);

    expect(panelIds).toContain(SESSION_ABC);
    expect(panelIds).toContain(SESSION_DEF);
    expect(panelIds).not.toContain("chat");
  });

  it("places side extras (files/changes/terminal) in the side column when one exists", () => {
    // Toggling plan mode ON: default layout has files/changes/terminal in
    // "right"; the plan preset has a "plan" column. They should land there,
    // not pile into center alongside the chat.
    const currentState = makeLayoutWithSide([{ id: SESSION_ABC, component: "chat" }], "right", [
      { id: "files", component: "files" },
      { id: "changes", component: "changes" },
      { id: "terminal-default", component: "terminal" },
    ]);
    const targetPreset = makeLayoutWithSide([{ id: "chat", component: "chat" }], "plan", [
      { id: "plan", component: "plan" },
    ]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    expect(panelIdsIn(result, "center")).toEqual([SESSION_ABC]);
    expect(panelIdsIn(result, "plan")).toEqual(["plan", "files", "changes", "terminal-default"]);
  });

  it("places the surviving plan panel in the center column, not the side column", () => {
    // Toggling plan mode OFF: the plan preset has plan in its own column,
    // which the default preset doesn't have. Plan should follow the chat
    // into center rather than being dumped into "right" alongside files.
    const currentState = makeLayoutWithSide([{ id: SESSION_ABC, component: "chat" }], "plan", [
      { id: "plan", component: "plan" },
    ]);
    const targetPreset = makeLayoutWithSide([{ id: "chat", component: "chat" }], "right", [
      { id: "files", component: "files" },
      { id: "changes", component: "changes" },
    ]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    expect(panelIdsIn(result, "center")).toEqual([SESSION_ABC, "plan"]);
    expect(panelIdsIn(result, "right")).toEqual(["files", "changes"]);
  });

  it("falls back to center for side extras when the target preset has no side column", () => {
    const currentState = makeLayoutWithSide([{ id: SESSION_ABC, component: "chat" }], "right", [
      { id: "files", component: "files" },
    ]);
    const targetPreset = makeLayout([{ id: "chat", component: "chat" }]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    expect(panelIdsIn(result, "center")).toEqual([SESSION_ABC, "files"]);
  });
});
