import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import { registerPluginTranslations } from "@/lib/plugins/plugin-translations";
import { pluginRegistry } from "@/lib/plugins/registry";
import {
  panel,
  REUSABLE_PANEL_IDS,
  type LayoutNode,
  type LayoutPanel,
  type LayoutState,
} from "@/lib/state/layout-manager";
import { LayoutEditor } from "./layout-editor";
import { resolveCustomLayoutPinnedWidths } from "@/lib/state/dockview-store";
import { toSerializedDockview } from "@/lib/state/layout-manager";
import {
  disposeDockview,
  mountDockview,
} from "@/lib/state/layout-manager/dockview-test-host.test-helpers";

const PLUGIN_ID = "kandev-plugin-prompt-history";
const PANEL_ID = `plugin:${PLUGIN_ID}:prompt-history`;
const CANONICAL_TITLE = "Prompt History";
const LOCALIZED_TITLE = "プロンプト履歴";

const pluginComponent = vi.fn(() => <div data-testid="real-plugin-content" />);

beforeEach(async () => {
  await i18n.changeLanguage("ja");
});

afterEach(async () => {
  cleanup();
  pluginRegistry.unregisterPlugin(PLUGIN_ID);
  pluginComponent.mockClear();
  await i18n.changeLanguage("en");
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

function registerPrompts(): void {
  registerPluginTranslations(PLUGIN_ID, {
    en: { panelTitle: CANONICAL_TITLE },
    ja: { panelTitle: LOCALIZED_TITLE },
  });
  pluginRegistry.forPlugin(PLUGIN_ID).registerTaskPanel({
    id: "prompt-history",
    title: CANONICAL_TITLE,
    titleKey: "panelTitle",
    Component: pluginComponent,
  });
}

function profileWithPlugin(): LayoutState {
  const pluginPanel = {
    id: PANEL_ID,
    component: "plugin-panel",
    title: CANONICAL_TITLE,
    tabComponent: "pluginPanelTab",
    params: { pluginId: PLUGIN_ID, panelKey: "prompt-history" },
  };
  return {
    columns: [
      {
        id: "center",
        width: 600,
        groups: [
          {
            id: "group-center",
            panels: [
              { id: "chat", component: "chat", title: "Agent" },
              { id: "plan", component: "plan", title: "Plan" },
            ],
            activePanel: "chat",
          },
        ],
      },
      {
        id: "right",
        width: 400,
        groups: [
          { id: "group-files", panels: [{ id: "files", component: "files", title: "Files" }] },
          { id: "group-plugin", panels: [pluginPanel], activePanel: PANEL_ID },
        ],
      },
    ],
  };
}

function mountEditor(layout: LayoutState, onChange = vi.fn()) {
  render(
    <TooltipProvider>
      <LayoutEditor layout={layout} editable onChange={onChange} />
    </TooltipProvider>,
  );
  return onChange;
}

function tabLabels(): string[] {
  return Array.from(document.querySelectorAll(".dv-default-tab")).map(
    (tab) => tab.textContent ?? "",
  );
}

function tabByLabel(label: string): HTMLElement {
  const tab = Array.from(document.querySelectorAll<HTMLElement>(".dv-default-tab")).find(
    (candidate) => candidate.textContent === label,
  );
  if (!tab) throw new Error(`no tab labelled ${label}; have ${tabLabels().join(", ")}`);
  return tab;
}

async function waitForEditor(): Promise<void> {
  await waitFor(() => expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`)).toBeTruthy());
}

function lastLayout(onChange: ReturnType<typeof vi.fn>): LayoutState {
  const calls = onChange.mock.calls;
  return calls[calls.length - 1][0] as LayoutState;
}

/** Select a tab, then nudge it one place left with the editor's own toolbar. */
async function moveTabLeft(label: string): Promise<void> {
  fireEvent.pointerDown(tabByLabel(label));
  fireEvent.click(tabByLabel(label));
  const toolbar = await screen.findByTestId("layout-editor-context-actions");
  fireEvent.click(within(toolbar).getAllByRole("button")[0]);
}

function pluginLeafTitles(layout: LayoutState): string[] {
  const column = layout.columns[1];
  const flat = column.groups.flatMap((group) => group.panels);
  const tree: LayoutNode[] = column.tree ? [column.tree] : [];
  const leaves: LayoutPanel[] = [];
  while (tree.length > 0) {
    const node = tree.pop()!;
    if (node.type === "leaf") leaves.push(...node.group.panels);
    else tree.push(...node.children);
  }
  return [...flat, ...leaves].filter((item) => item.id === PANEL_ID).map((item) => item.title);
}

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.5, AC-UI-TASK-LAYOUT-PROFILES-002.6, AC-UI-TASK-LAYOUT-PROFILES-002.8 */
describe("LayoutEditor plugin panels", () => {
  it("renders only a placeholder, never the plugin component", async () => {
    registerPrompts();
    mountEditor(profileWithPlugin());

    await waitForEditor();

    expect(pluginComponent).not.toHaveBeenCalled();
    expect(screen.queryByTestId("real-plugin-content")).toBeNull();
  });

  it("shows the retained canonical title without dirtying the profile after revocation", async () => {
    registerPrompts();
    const onChange = mountEditor(profileWithPlugin());
    await waitForEditor();
    expect(tabLabels()).toContain(LOCALIZED_TITLE);
    expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(LOCALIZED_TITLE);
    onChange.mockClear();

    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

    await waitFor(() => expect(tabLabels()).toContain(CANONICAL_TITLE));
    expect(tabLabels()).not.toContain(LOCALIZED_TITLE);
    await waitFor(() =>
      expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(
        CANONICAL_TITLE,
      ),
    );
    expect(onChange).not.toHaveBeenCalled();
  });

  it("saves the canonical title when another tab is edited after the registration is revoked", async () => {
    registerPrompts();
    const onChange = mountEditor(profileWithPlugin());
    await waitForEditor();
    act(() => pluginRegistry.unregisterPlugin(PLUGIN_ID));

    await moveTabLeft("計画");

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    const saved = lastLayout(onChange);
    expect(saved.columns[0].groups[0].panels.map((item) => item.id)).toEqual(["plan", "chat"]);
    expect(pluginLeafTitles(saved)).toEqual(expect.arrayContaining([CANONICAL_TITLE]));
    expect(pluginLeafTitles(saved).every((title) => title === CANONICAL_TITLE)).toBe(true);
    expect(saved.columns[1].groups[1].panels[0]).toEqual({
      id: PANEL_ID,
      component: "plugin-panel",
      title: CANONICAL_TITLE,
      tabComponent: "pluginPanelTab",
      params: { pluginId: PLUGIN_ID, panelKey: "prompt-history" },
    });
  });

  it("falls back to the saved title for a profile opened before the plugin is available", async () => {
    const onChange = mountEditor(profileWithPlugin());
    await waitForEditor();
    expect(tabLabels()).toContain(CANONICAL_TITLE);
    expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(CANONICAL_TITLE);

    await moveTabLeft("計画");

    await waitFor(() => expect(onChange).toHaveBeenCalled());
    expect(pluginLeafTitles(lastLayout(onChange)).every((title) => title === CANONICAL_TITLE)).toBe(
      true,
    );
    onChange.mockClear();

    act(() => registerPrompts());

    await waitFor(() => expect(tabLabels()).toContain(LOCALIZED_TITLE));
    await waitFor(() =>
      expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(
        LOCALIZED_TITLE,
      ),
    );
    expect(onChange).not.toHaveBeenCalled();
  });

  it("updates the visible placeholder and tab on locale changes without editing the layout", async () => {
    registerPrompts();
    const onChange = mountEditor(profileWithPlugin());
    await waitForEditor();
    expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(LOCALIZED_TITLE);
    onChange.mockClear();

    await act(async () => {
      await i18n.changeLanguage("en");
    });

    await waitFor(() => expect(tabLabels()).toContain(CANONICAL_TITLE));
    await waitFor(() =>
      expect(screen.getByTestId(`layout-placeholder-${PANEL_ID}`).textContent).toBe(
        CANONICAL_TITLE,
      ),
    );
    expect(onChange).not.toHaveBeenCalled();
    expect(pluginComponent).not.toHaveBeenCalled();
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("LayoutEditor source geometry", () => {
  it.each([
    {
      orientation: "HORIZONTAL" as const,
      widths: [960, 240],
      preview: 1200,
      task: 1200,
      right: 240,
    },
    {
      orientation: "HORIZONTAL" as const,
      widths: [960, 240],
      preview: 600,
      task: 1200,
      right: 240,
    },
    {
      orientation: "HORIZONTAL" as const,
      widths: [600, 600],
      preview: 800,
      task: 1600,
      right: 800,
    },
    { orientation: "HORIZONTAL" as const, widths: [950, 50], preview: 600, task: 4000, right: 200 },
    {
      orientation: "HORIZONTAL" as const,
      widths: [undefined, 200],
      preview: 600,
      task: 1600,
      right: 200,
    },
    { orientation: "VERTICAL" as const, widths: [250, 350], preview: 1200, task: 1200, right: 350 },
    { orientation: "HORIZONTAL" as const, widths: [], preview: 1200, task: 1200, right: 360 },
  ])(
    "preserves $orientation source $widths through a tab edit in a $preview px preview",
    async ({ orientation, widths, preview, task, right }) => {
      vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
        new DOMRect(0, 0, preview, 600),
      );
      const savedProfile = profileWithPlugin();
      savedProfile.rootOrientation = orientation;
      savedProfile.columns[1].pinned = true;
      savedProfile.columns.forEach((column, index) => {
        if (widths[index] === undefined) delete column.width;
        else column.width = widths[index];
      });
      const onChange = mountEditor(savedProfile);
      await waitForEditor();

      await moveTabLeft("計画");

      await waitFor(() => expect(onChange).toHaveBeenCalled());
      const saved = lastLayout(onChange);
      expect(saved.columns.map((column) => column.width)).toEqual(
        widths.length ? widths : [840, 360],
      );
      expect(saved.rootOrientation).toBe(orientation);
      expect(saved.columns[0].groups[0].panels.map((item) => item.id)).toEqual(["plan", "chat"]);
      expect(saved.columns[1].groups[1].panels[0].id).toBe(PANEL_ID);
      cleanup();
      const reloaded = JSON.parse(JSON.stringify(saved)) as LayoutState;
      const afterReload = mountEditor(reloaded);
      await waitForEditor();
      await moveTabLeft("エージェント");
      await waitFor(() => expect(afterReload).toHaveBeenCalled());
      const resaved = lastLayout(afterReload);
      vi.stubGlobal("innerWidth", task);
      const live = mountDockview(task, 600);
      try {
        live.fromJSON(
          toSerializedDockview(
            resaved,
            task,
            600,
            resolveCustomLayoutPinnedWidths(resaved.columns, task, orientation),
          ),
        );
        expect(
          live.getPanel(PANEL_ID)?.group.api[orientation === "VERTICAL" ? "height" : "width"],
        ).toBe(right);
      } finally {
        disposeDockview(live);
      }
    },
  );
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe("LayoutEditor intentional geometry edits", () => {
  it.each(["resize", "split"])(
    "saves an intentional %s instead of retaining old geometry",
    async (action) => {
      await i18n.changeLanguage("en");
      vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue(
        new DOMRect(0, 0, 600, 600),
      );
      const profile = profileWithPlugin();
      profile.columns[0].width = 960;
      profile.columns[1].width = 240;
      profile.columns[1].pinned = true;
      const onChange = mountEditor(profile);
      await waitForEditor();
      fireEvent.pointerDown(tabByLabel("Plan"));
      fireEvent.click(tabByLabel("Plan"));
      const toolbar = await screen.findByTestId("layout-editor-context-actions");
      fireEvent.pointerDown(
        within(toolbar).getByRole("button", {
          name: action === "resize" ? "Arrange split" : "Split tab",
        }),
        { button: 0, ctrlKey: false },
      );
      fireEvent.click(
        await screen.findByRole("menuitem", {
          name: action === "resize" ? "Increase width" : "Below",
        }),
      );
      await waitFor(() => expect(onChange).toHaveBeenCalled());
      const saved = lastLayout(onChange);
      expect(saved.columns.map((column) => column.width)).toEqual(
        action === "resize" ? [460, 140] : [420, 180],
      );
      expect(saved.columns[0].groups.map((group) => group.panels.map((panel) => panel.id))).toEqual(
        action === "resize" ? [["chat", "plan"]] : [["chat"], ["plan"]],
      );
    },
  );
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.5 */
describe("LayoutEditor built-in panels", () => {
  it("renders a placeholder for every reusable panel the Add panel menu can offer", async () => {
    const layout: LayoutState = {
      columns: REUSABLE_PANEL_IDS.map((id) => ({
        id: `column-${id}`,
        groups: [{ id: `group-${id}`, panels: [panel(id)], activePanel: id }],
      })),
    };

    mountEditor(layout);

    await waitFor(() => {
      for (const id of REUSABLE_PANEL_IDS) {
        expect(screen.getByTestId(`layout-placeholder-${id}`)).toBeTruthy();
      }
    });
  });
});
