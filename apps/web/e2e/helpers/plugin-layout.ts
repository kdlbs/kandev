import { expect, type Page } from "@playwright/test";
import { PLUGIN_ID } from "./plugin-fixture";

export const NOTES_ID = `plugin:${PLUGIN_ID}:notes`;
export const PROMPT_ID = `plugin:${PLUGIN_ID}:prompt-history-plugin`;
export const NOTES_TITLE = "Notes";
export const PROMPT_TITLE = "Prompt history fixture";
export const PROMPT_PSEUDO_TITLE = "Ƥřǿɱƥŧ ħīşŧǿřẏ ƒīẋŧŭřḗ";

type LayoutPanel = {
  id: string;
  component: string;
  title: string;
  tabComponent?: string;
  params?: Record<string, unknown>;
};

export function pluginPanel(id: string, title: string): LayoutPanel {
  const panelKey = id.slice(`plugin:${PLUGIN_ID}:`.length);
  return {
    id,
    component: "plugin-panel",
    title,
    tabComponent: "pluginPanelTab",
    params: { pluginId: PLUGIN_ID, panelKey },
  };
}

const AGENT: LayoutPanel = {
  id: "chat",
  component: "chat",
  title: "Agent",
  tabComponent: "permanentTab",
};
const FILES: LayoutPanel = { id: "files", component: "files", title: "Files" };
const CHANGES: LayoutPanel = {
  id: "changes",
  component: "changes",
  title: "Changes",
  tabComponent: "changesTab",
};

/**
 * Agent in the centre column and a canonical right column: Files/Changes over
 * `bottom`. `ratio` is the saved centre/right share (the right column is in cap
 * and not the stock 30%); omitting `rootOrientation` is the legacy horizontal form.
 */
export function horizontalPluginLayout(
  bottom: LayoutPanel,
  ratio: { center: number; right: number } = { center: 800, right: 200 },
) {
  return {
    columns: [
      {
        id: "center",
        width: ratio.center,
        groups: [{ id: "group-center", panels: [AGENT], activePanel: "chat" }],
      },
      {
        id: "right",
        pinned: true,
        width: ratio.right,
        groups: [
          { id: "group-right-top", panels: [FILES, CHANGES], activePanel: "files" },
          { id: "group-right-bottom", panels: [bottom], activePanel: bottom.id },
        ],
      },
    ],
  };
}

/** Agent beside one Files/Changes group that also holds `plugin`; the legacy form with no orientation. */
export function filesGroupPluginLayout(plugin: LayoutPanel) {
  const layout = horizontalPluginLayout(plugin);
  const right = layout.columns[1];
  const [top] = right.groups;
  top.panels.push(plugin);
  right.groups = [top];
  return layout;
}

/** A VERTICAL root: a short Agent row above a taller row split into Files/Changes beside Prompt history. */
export function verticalPluginLayout() {
  const files = { id: "group-files", panels: [FILES, CHANGES], activePanel: "changes" };
  const prompt = {
    id: "group-prompt",
    panels: [pluginPanel(PROMPT_ID, PROMPT_TITLE)],
    activePanel: PROMPT_ID,
  };
  return {
    rootOrientation: "VERTICAL",
    columns: [
      {
        id: "center",
        width: 300,
        groups: [{ id: "group-center", panels: [AGENT], activePanel: "chat" }],
      },
      {
        // Historical snapshots carry horizontal pin metadata on this row.
        id: "right",
        pinned: true,
        width: 400,
        groups: [files, prompt],
        tree: {
          type: "branch",
          size: 400,
          children: [
            { type: "leaf", size: 400, group: files },
            { type: "leaf", size: 600, group: prompt },
          ],
        },
      },
    ],
  };
}

export function savedProfile(id: string, name: string, layout: unknown, isDefault = true) {
  return {
    id,
    name,
    is_default: isDefault,
    layout: layout as Record<string, unknown>,
    created_at: new Date().toISOString(),
  };
}

type RootWindow = Window & {
  __dockviewApi__?: {
    component?: { gridview?: { root?: { splitview?: RootSplitview } } };
    getPanel: (id: string) => PanelLike | undefined;
  };
};
type RootSplitview = {
  length: number;
  getViewSize: (index: number) => number;
  sashes: Array<{ container: HTMLElement }>;
};
type PanelLike = {
  group: { id: string; width: number; height: number; panels: Array<{ id: string }> };
  api: { isActive: boolean };
};

/** Extents of the root split's children along its own axis (heights for a VERTICAL root). */
export async function rootSizes(page: Page): Promise<number[]> {
  return page.evaluate(() => {
    const splitview = (window as RootWindow).__dockviewApi__?.component?.gridview?.root?.splitview;
    if (!splitview) throw new Error("dockview root splitview not available");
    return Array.from({ length: splitview.length }, (_, index) => splitview.getViewSize(index));
  });
}

export type PanelPlacement = {
  groupId: string;
  groupPanelIds: string[];
  active: boolean;
  width: number;
  height: number;
};

export async function panelPlacement(page: Page, panelId: string): Promise<PanelPlacement | null> {
  return page.evaluate((id) => {
    const panel = (window as RootWindow).__dockviewApi__?.getPanel(id);
    if (!panel) return null;
    return {
      groupId: panel.group.id,
      groupPanelIds: panel.group.panels.map((item) => item.id),
      active: panel.api.isActive,
      width: panel.group.width,
      height: panel.group.height,
    };
  }, panelId);
}

/**
 * Drag a root sash the way the app's own resize specs do: press it, move the
 * neighbouring root child to `target` along the root axis, and release. A real
 * pointer drag is not used because production widens the pinned column's cap
 * only on the mousedown the splitview never sees from synthetic pointer input.
 */
export async function dragRootSash(
  page: Page,
  sash: "first" | "last",
  childIndex: number,
  target: number,
): Promise<void> {
  await page.evaluate(
    ({ which, index, size }) => {
      const splitview = (window as RootWindow).__dockviewApi__?.component?.gridview?.root
        ?.splitview as
        | (RootSplitview & { resizeView: (index: number, size: number) => void })
        | undefined;
      if (!splitview) throw new Error("dockview root splitview not available");
      const target =
        splitview.sashes[which === "first" ? 0 : splitview.sashes.length - 1]?.container;
      if (!target) throw new Error("root sash not available");
      target.dispatchEvent(new MouseEvent("mousedown", { button: 0, bubbles: true }));
      splitview.resizeView(index, size);
      target.dispatchEvent(new MouseEvent("mouseup", { button: 0, bubbles: true }));
    },
    { which: sash, index: childIndex, size: target },
  );
}

/** True when any task environment has a manual right width recorded in this tab. */
export async function hasManualRightWidth(page: Page): Promise<boolean> {
  return page.evaluate(() =>
    Object.keys(window.sessionStorage).some((key) =>
      key.startsWith("kandev.dockview.manual-right-width-v1."),
    ),
  );
}

export async function setLocale(page: Page, locale: "en" | "pseudo"): Promise<void> {
  await page.evaluate((value) => {
    document.cookie = `kandev_locale=${value}; path=/; max-age=31536000; SameSite=Lax`;
  }, locale);
}

/** Drop the plugin's registration from the running page, as disabling it would. */
export async function unloadPluginInPage(page: Page): Promise<void> {
  await page.evaluate((pluginId) => {
    const host = (
      window as Window & {
        __KANDEV_E2E_PLUGIN_HOST__?: { unloadPlugin: (id: string) => void };
      }
    ).__KANDEV_E2E_PLUGIN_HOST__;
    if (!host) throw new Error("plugin E2E host is not exposed");
    host.unloadPlugin(pluginId);
  }, PLUGIN_ID);
}

/** Expect `actual` to be within `tolerance` of `expected` (fractions of a whole). */
export function expectRatio(actual: number, whole: number, expected: number, tolerance = 0.04) {
  expect(Math.abs(actual / whole - expected)).toBeLessThanOrEqual(tolerance);
}
