import { expect, it } from "vitest";

import { materializeReusableChatPanel, normalizeReusableSessionPanels } from "./session-panels";
import type { LayoutGroup, LayoutPanel, LayoutState } from "./types";

const CENTER_GROUP_ID = "group-center";
const OLD_SESSION_ID = "s-old";
const NEW_SESSION_ID = "s-new";
const OLD_SESSION_PANEL_ID = `session:${OLD_SESSION_ID}`;
const NEW_SESSION_PANEL_ID = `session:${NEW_SESSION_ID}`;
const CHAT_ID = "chat";
const CHAT_COMPONENT = "chat";

function panel(id: string, component = id): LayoutPanel {
  return { id, component, title: id };
}

function chatPlaceholder(): LayoutPanel {
  return { id: CHAT_ID, component: CHAT_COMPONENT, title: "Agent", tabComponent: "permanentTab" };
}

function oldSessionPanel(id = OLD_SESSION_PANEL_ID): LayoutPanel {
  return {
    id,
    component: CHAT_COMPONENT,
    title: "Old task",
    tabComponent: "sessionTab",
    params: { sessionId: OLD_SESSION_ID },
  };
}

function layoutWithGroup(group: LayoutGroup): LayoutState {
  return {
    columns: [
      {
        id: "center",
        groups: [group],
      },
    ],
  };
}

it("normalizes saved session chat panels to a reusable chat placeholder", () => {
  const normalized = normalizeReusableSessionPanels(
    layoutWithGroup({
      id: CENTER_GROUP_ID,
      activePanel: OLD_SESSION_PANEL_ID,
      panels: [oldSessionPanel(), panel("changes")],
    }),
  );

  expect(normalized.columns[0]?.groups[0]?.activePanel).toBe(CHAT_ID);
  expect(normalized.columns[0]?.groups[0]?.panels).toEqual([chatPlaceholder(), panel("changes")]);
});

it("materializes reusable chat panels to the active task session", () => {
  const materialized = materializeReusableChatPanel(
    layoutWithGroup({
      id: CENTER_GROUP_ID,
      activePanel: CHAT_ID,
      panels: [chatPlaceholder(), panel("plan")],
    }),
    NEW_SESSION_ID,
  );

  expect(materialized.columns[0]?.groups[0]?.activePanel).toBe(NEW_SESSION_PANEL_ID);
  expect(materialized.columns[0]?.groups[0]?.panels[0]).toEqual({
    id: NEW_SESSION_PANEL_ID,
    component: CHAT_COMPONENT,
    title: "Agent",
    tabComponent: "sessionTab",
    params: { sessionId: NEW_SESSION_ID },
  });
});

it("keeps tree and flat groups in sync when rewriting session panels", () => {
  const materialized = materializeReusableChatPanel(
    {
      columns: [
        {
          id: "center",
          groups: [],
          tree: {
            type: "leaf",
            group: {
              id: CENTER_GROUP_ID,
              activePanel: OLD_SESSION_PANEL_ID,
              panels: [oldSessionPanel()],
            },
          },
        },
      ],
    },
    NEW_SESSION_ID,
  );
  const group = materialized.columns[0]?.groups[0];

  expect(group?.activePanel).toBe(NEW_SESSION_PANEL_ID);
  expect(group?.panels[0]?.id).toBe(NEW_SESSION_PANEL_ID);
  expect(materialized.columns[0]?.tree).toMatchObject({
    type: "leaf",
    group: {
      activePanel: NEW_SESSION_PANEL_ID,
      panels: [{ id: NEW_SESSION_PANEL_ID }],
    },
  });
});

it("drops duplicate chat session panels from reusable layouts", () => {
  const materialized = materializeReusableChatPanel(
    layoutWithGroup({
      id: CENTER_GROUP_ID,
      activePanel: OLD_SESSION_PANEL_ID,
      panels: [oldSessionPanel(), oldSessionPanel("session:s-other"), panel("plan")],
    }),
    NEW_SESSION_ID,
  );

  expect(materialized.columns[0]?.groups[0]?.panels.map((item) => item.id)).toEqual([
    NEW_SESSION_PANEL_ID,
    "plan",
  ]);
});

it("deduplicates session chat panels across columns", () => {
  const materialized = materializeReusableChatPanel(
    {
      columns: [
        {
          id: "center",
          groups: [{ id: CENTER_GROUP_ID, panels: [oldSessionPanel()] }],
        },
        {
          id: "right",
          groups: [
            { id: "group-right", panels: [oldSessionPanel("session:s-other"), panel("plan")] },
          ],
        },
      ],
    },
    NEW_SESSION_ID,
  );

  expect(materialized.columns.map((column) => column.id)).toEqual(["center", "right"]);
  expect(materialized.columns[0]?.groups[0]?.panels.map((item) => item.id)).toEqual([
    NEW_SESSION_PANEL_ID,
  ]);
  expect(materialized.columns[1]?.groups[0]?.panels.map((item) => item.id)).toEqual(["plan"]);
});
