import { describe, expect, it } from "vitest";
import type { DockviewApi } from "dockview-react";
import { filterEphemeral, fromDockviewApi, toSerializedDockview } from "./serializer";
import { panel, PROMPT_HISTORY_PANEL_ID, RIGHT_BOTTOM_GROUP, RIGHT_TOP_GROUP } from "./constants";
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

type CapturedGroup = {
  id: string;
  panels: Array<{ id: string; component: string; title?: string }>;
};

function makeCapturedApi(columns: CapturedGroup[][]): DockviewApi {
  const leaf = (group: CapturedGroup) => ({
    view: {
      id: group.id,
      panels: group.panels.map((item) => ({
        id: item.id,
        title: item.title ?? item.id,
        view: { contentComponent: item.component },
      })),
      activePanel: group.panels[0] ? { id: group.panels[0].id } : undefined,
    },
  });
  const rootChildren = columns.map((groups) =>
    groups.length === 1
      ? leaf(groups[0])
      : {
          children: groups.map(leaf),
          splitview: { getViewSize: (index: number) => 300 + index * 10 },
        },
  );
  return {
    component: {
      gridview: {
        root: {
          children: rootChildren,
          splitview: { getViewSize: (index: number) => 700 - index * 300 },
        },
      },
    },
  } as unknown as DockviewApi;
}

describe("fromDockviewApi — right-column ownership", () => {
  it("captures a live Agent session as the compact center column", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [
          {
            id: "runtime-center",
            panels: [
              { id: "session:session-a", component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
              { id: "changes", component: "changes", title: "Changes" },
              { id: "terminal-default", component: "terminal", title: "Terminal" },
            ],
          },
        ],
      ]),
    );

    expect(captured.columns).toHaveLength(1);
    expect(captured.columns[0]?.id).toBe("center");
    expect(captured.columns[0]?.pinned).toBeUndefined();

    const serialized = toSerializedDockview(captured, 1000, 800, new Map());
    const root = (serialized as unknown as { grid: { root: { data: unknown[] } } }).grid.root;
    expect((root.data[0] as { data: { id: string } }).data.id).toBe("runtime-center");
  });

  it("keeps a mixed session center separate from a nested right column", () => {
    const captured = fromDockviewApi(
      makeCapturedApi([
        [
          {
            id: "runtime-agent",
            panels: [
              { id: "session:session-a", component: "chat", title: "Agent" },
              { id: "files", component: "files", title: "Files" },
              { id: "pr-detail", component: "pr-detail", title: "PR Details" },
            ],
          },
        ],
        [
          { id: RIGHT_TOP_GROUP, panels: [{ id: "files", component: "files" }] },
          { id: RIGHT_BOTTOM_GROUP, panels: [{ id: "terminal-default", component: "terminal" }] },
        ],
      ]),
    );

    expect(captured.columns.map((column) => column.id)).toEqual(["center", "right"]);
    expect(captured.columns[0]?.groups[0]?.panels.map((item) => item.id)).toEqual([
      "session:session-a",
      "files",
      "pr-detail",
    ]);
    expect(captured.columns[1]?.groups.map((group) => group.id)).toEqual([
      RIGHT_TOP_GROUP,
      RIGHT_BOTTOM_GROUP,
    ]);
  });
});

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
