import { vi } from "vitest";
import type { DockviewApi } from "dockview-react";
import { CENTER_GROUP, RIGHT_TOP_GROUP } from "@/lib/state/layout-manager";

type HandoffPanel = {
  id: string;
  group: HandoffGroup;
  api: {
    close: ReturnType<typeof vi.fn<() => void>>;
    component: string;
  };
};

type HandoffGroup = {
  id: string;
  panels: HandoffPanel[];
};

type HandoffAddOptions = {
  id: string;
  component: string;
  position?: { referenceGroup?: string; index?: number };
};

/**
 * Models the Dockview behavior behind the layout corruption: closing the
 * final panel synchronously destroys its group. The right-side groups remain
 * so a later fallback would attach the incoming session in the wrong split.
 */
export function makeSharedEnvironmentHandoffApi(): {
  api: DockviewApi;
  events: string[];
  groupIds: () => string[];
} {
  const center: HandoffGroup = { id: CENTER_GROUP, panels: [] };
  const rightTop: HandoffGroup = { id: RIGHT_TOP_GROUP, panels: [] };
  const rightBottom: HandoffGroup = { id: "group-right-bottom", panels: [] };
  const groups = [center, rightTop, rightBottom];
  const panels: HandoffPanel[] = [];
  const events: string[] = [];

  const removePanel = (panel: HandoffPanel) => {
    events.push(`close:${panel.id}`);
    const panelIndex = panels.indexOf(panel);
    if (panelIndex !== -1) panels.splice(panelIndex, 1);
    const groupPanelIndex = panel.group.panels.indexOf(panel);
    if (groupPanelIndex !== -1) panel.group.panels.splice(groupPanelIndex, 1);
    if (panel.group.panels.length === 0) {
      const groupIndex = groups.indexOf(panel.group);
      if (groupIndex !== -1) groups.splice(groupIndex, 1);
    }
  };

  const createPanel = (id: string, component: string, group: HandoffGroup, index?: number) => {
    const panel: HandoffPanel = {
      id,
      group,
      api: {
        close: vi.fn(() => removePanel(panel)),
        component,
      },
    };
    panels.push(panel);
    group.panels.splice(index ?? group.panels.length, 0, panel);
  };

  createPanel("session:outgoing", "chat", center);
  createPanel("files", "files", rightTop);
  createPanel("changes", "changes", rightTop);
  createPanel("terminal-default", "terminal", rightBottom);

  return {
    api: {
      panels,
      groups,
      getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
      addPanel: (options: HandoffAddOptions) => {
        const group = groups.find((candidate) => candidate.id === options.position?.referenceGroup);
        if (!group) throw new Error("expected a live reference group");
        events.push(`add:${options.id}`);
        createPanel(options.id, options.component, group, options.position?.index);
      },
    } as unknown as DockviewApi,
    events,
    groupIds: () => groups.map((group) => group.id),
  };
}
