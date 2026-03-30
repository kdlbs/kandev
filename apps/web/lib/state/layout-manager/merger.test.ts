import { describe, it, expect } from "vitest";
import type { LayoutState } from "./types";
import { mergePanelsIntoPreset } from "./merger";

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
      { id: "session:abc", component: "chat" },
      { id: "session:def", component: "chat" },
    ]);
    const targetPreset = makeLayout([{ id: "chat", component: "chat" }]);

    const result = mergePanelsIntoPreset(currentState, targetPreset);

    const centerPanels = result.columns.find((c) => c.id === "center")!.groups[0].panels;
    const panelIds = centerPanels.map((p) => p.id);

    expect(panelIds).toContain("session:abc");
    expect(panelIds).toContain("session:def");
    expect(panelIds).not.toContain("chat");
  });
});
