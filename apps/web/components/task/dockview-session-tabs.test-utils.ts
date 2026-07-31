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

type ReorderingMoveOptions = {
  index: number;
  skipSetActive: boolean;
};

type ReorderingPanel = {
  id: string;
  group: ReorderingGroup;
  api: {
    close: ReturnType<typeof vi.fn<() => void>>;
    component: string;
    moveTo: ReturnType<typeof vi.fn<(options: ReorderingMoveOptions) => void>>;
    setActive: ReturnType<typeof vi.fn<() => void>>;
  };
};

type ReorderingGroup = {
  id: string;
  panels: ReorderingPanel[];
  activePanel: ReorderingPanel | null;
};

/**
 * Models Dockview 4.13.1's separate group-local selection and global active
 * panel, including the sibling activation caused by removing or moving the
 * selected panel.
 */
export function makeReorderingAutoSessionApi(globalActivePanelId: "chat" | "files" = "chat"): {
  api: DockviewApi;
  activePanelId: () => string | null;
  centerActivePanelId: () => string | null;
  activationSequence: string[];
} {
  const panels: ReorderingPanel[] = [];
  const group: ReorderingGroup = { id: CENTER_GROUP, panels: [], activePanel: null };
  const rightGroup: ReorderingGroup = { id: RIGHT_TOP_GROUP, panels: [], activePanel: null };
  const activationSequence: string[] = [];
  let activePanel: ReorderingPanel | null = null;

  const activate = (panel: ReorderingPanel) => {
    panel.group.activePanel = panel;
    activePanel = panel;
    activationSequence.push(panel.id);
  };

  const removePanel = (panel: ReorderingPanel) => {
    const panelGroup = panel.group;
    const groupIndex = panelGroup.panels.indexOf(panel);
    if (groupIndex !== -1) panelGroup.panels.splice(groupIndex, 1);
    const panelIndex = panels.indexOf(panel);
    if (panelIndex !== -1) panels.splice(panelIndex, 1);
    if (panelGroup.activePanel === panel) {
      const successor = panelGroup.panels[0] ?? null;
      panelGroup.activePanel = successor;
      if (successor) activate(successor);
    }
  };

  const createPanel = (
    id: string,
    component: string,
    panelGroup: ReorderingGroup = group,
    index?: number,
  ): ReorderingPanel => {
    const panel: ReorderingPanel = {
      id,
      group: panelGroup,
      api: {
        close: vi.fn(() => removePanel(panel)),
        component,
        moveTo: vi.fn((options: ReorderingMoveOptions) => {
          const sourceIndex = panelGroup.panels.indexOf(panel);
          if (sourceIndex !== -1) panelGroup.panels.splice(sourceIndex, 1);
          if (panelGroup.activePanel === panel) {
            const successor = panelGroup.panels[0] ?? null;
            panelGroup.activePanel = successor;
            if (successor) activate(successor);
          }
          panelGroup.panels.splice(options.index, 0, panel);
          if (!options.skipSetActive) activate(panel);
        }),
        setActive: vi.fn(() => activate(panel)),
      },
    };
    panels.push(panel);
    panelGroup.panels.splice(index ?? panelGroup.panels.length, 0, panel);
    return panel;
  };

  const chatPanel = createPanel("chat", "chat");
  createPanel("plan", "plan");
  const filesPanel = createPanel("files", "files", rightGroup);
  group.activePanel = chatPanel;
  rightGroup.activePanel = filesPanel;
  activePanel = globalActivePanelId === "files" ? filesPanel : chatPanel;

  const api = {
    get activePanel() {
      return activePanel;
    },
    panels,
    groups: [group, rightGroup],
    getPanel: (id: string) => panels.find((panel) => panel.id === id) ?? null,
    addPanel: (options: {
      id: string;
      component: string;
      inactive?: boolean;
      position?: { index?: number };
    }): ReorderingPanel => {
      const panel = createPanel(options.id, options.component, group, options.position?.index);
      if (!options.inactive) activate(panel);
      return panel;
    },
    removePanel,
  } as unknown as DockviewApi;

  return {
    api,
    activePanelId: () => activePanel?.id ?? null,
    centerActivePanelId: () => group.activePanel?.id ?? null,
    activationSequence,
  };
}
