import { describe, expect, it } from "vitest";
import { filterEphemeral, toSerializedDockview } from "./serializer";
import { panel, PROMPT_HISTORY_PANEL_ID } from "./constants";
import type { LayoutPanel, LayoutState } from "./types";

/** Builds an ephemeral LayoutPanel whose title is its id. */
function ephemeralPanel(id: string, component: string): LayoutPanel {
  return { id, component, title: id };
}

/**
 * Capture/restore survival for the reusable `prompt-history` panel, mirroring
 * the Todos registration. Without membership in `KNOWN_PANEL_IDS`,
 * `filterEphemeral` drops the panel from captured layouts and restore skips
 * canonical-title normalization (`serializer.ts` keys off that set).
 */
function layoutWithPromptHistory(extraPanels: LayoutPanel[] = []): LayoutState {
  return {
    columns: [
      {
        id: "center",
        groups: [
          {
            id: "group-center",
            panels: [panel(PROMPT_HISTORY_PANEL_ID), ...extraPanels],
          },
        ],
      },
    ],
  } as unknown as LayoutState;
}

describe("filterEphemeral — prompt-history survival", () => {
  it("keeps the prompt-history panel when filtering a captured layout", () => {
    const filtered = filterEphemeral(layoutWithPromptHistory());

    const groups = filtered.columns.flatMap((column) => column.groups);
    const ids = groups.flatMap((group) => group.panels.map((p) => p.id));
    expect(ids).toContain(PROMPT_HISTORY_PANEL_ID);
  });

  it("keeps prompt-history next to other reusable panels and drops ephemeral ones", () => {
    const filtered = filterEphemeral(
      layoutWithPromptHistory([
        panel("todos"),
        ephemeralPanel("file:src/app.ts", "file-editor"),
        ephemeralPanel("diff:file:src/app.ts", "diff-viewer"),
      ]),
    );

    const groups = filtered.columns.flatMap((column) => column.groups);
    const ids = groups.flatMap((group) => group.panels.map((p) => p.id));
    expect(ids).toContain(PROMPT_HISTORY_PANEL_ID);
    expect(ids).toContain("todos");
    expect(ids).not.toContain("file:src/app.ts");
    expect(ids).not.toContain("diff:file:src/app.ts");
  });

  it("normalizes the stored prompt-history title to canonical English", () => {
    const filtered = filterEphemeral(layoutWithPromptHistory());

    const groups = filtered.columns.flatMap((column) => column.groups);
    const promptHistory = groups
      .flatMap((group) => group.panels)
      .find((p) => p.id === PROMPT_HISTORY_PANEL_ID);
    expect(promptHistory?.title).toBe("Prompt History");
  });
});

describe("toSerializedDockview — prompt-history title normalization", () => {
  it("serializes the prompt-history panel with its localized registry title", () => {
    const serialized = toSerializedDockview(layoutWithPromptHistory(), 1000, 800, new Map());
    const panels = (serialized as unknown as { panels: Record<string, { title: string }> }).panels;

    expect(panels[PROMPT_HISTORY_PANEL_ID].title).toBe("Prompt history");
  });
});
