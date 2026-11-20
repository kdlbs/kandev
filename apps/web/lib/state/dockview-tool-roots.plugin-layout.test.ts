import { requireRootSplitview as getRootSplitview } from "./layout-manager/dockview-test-host.test-helpers";
import { describe, expect, it, vi } from "vitest";
import {
  mergeGroup,
  movePanelToGroup,
  splitPanel,
} from "@/components/settings/layouts/layout-editor-actions";
import {
  setupGroupTracking,
  setupSashDragCapToggle,
} from "@/components/task/dockview-layout-setup";
import { validateReusableLayout } from "@/lib/layout/layout-profiles";
import { getManualRightWidth, setManualRightWidth } from "@/lib/local-storage";
import { useDockviewStore } from "./dockview-store";
import {
  applyLayout,
  clearAllPinnedTargets,
  fromDockviewApi,
  type LayoutState,
} from "./layout-manager";
import { clearReusableRootGeometry } from "./layout-manager/pinned-targets";
import {
  ENV,
  HEIGHT,
  CHAT,
  FILES,
  PLUGIN,
  stops,
  mount,
  settle,
  expectGeometry,
  sourceFromSettings,
  build,
  resize,
  setupRootGeometryTests,
} from "@/components/task/dockview-root-geometry.test-helpers";

vi.mock("@/lib/layout/panel-portal-manager", () => ({
  panelPortalManager: { releaseByEnv: vi.fn(), reconcile: vi.fn() },
}));
setupRootGeometryTests();

const TOOL_ROOT_CASES = {
  changes: {
    panel: "changes",
    separatePlugin: false,
    wide: [400, 800, 400],
    narrow: [240, 320, 240],
    pinnedPanel: "files",
    flexiblePanel: "changes",
  },
  files: {
    panel: "files",
    separatePlugin: false,
    wide: [400, 400, 800],
    narrow: [200, 200, 400],
    pinnedPanel: "changes",
    flexiblePanel: "files",
  },
  fourRoots: {
    panel: "changes",
    separatePlugin: true,
    wide: [400, 600, 400, 200],
    narrow: [200, 300, 200, 100],
    pinnedPanel: "files",
    flexiblePanel: "changes",
  },
} as const;

function sourceFromToolSplit(split: keyof typeof TOOL_ROOT_CASES, capture: boolean) {
  const editor = mount(1600);
  applyLayout(
    editor,
    {
      columns: [
        { id: "center", width: 400, groups: [{ panels: [CHAT] }] },
        {
          id: "right",
          pinned: true,
          width: 1200,
          groups: [
            {
              id: "group-right-top",
              panels: [FILES, { id: "changes", component: "changes", title: "Changes" }, PLUGIN],
            },
          ],
        },
      ],
    },
    new Map([["right", 1120]]),
    1600,
    HEIGHT,
  );
  const geometry = TOOL_ROOT_CASES[split];
  if (geometry.separatePlugin) expect(splitPanel(editor, PLUGIN.id, "right")).toBe(true);
  expect(splitPanel(editor, geometry.panel, "right")).toBe(true);
  const { wide } = geometry;
  wide.forEach((size, index) => getRootSplitview(editor).resizeView(index, size));
  expectGeometry(editor, wide);
  useDockviewStore.setState({ api: editor, preMaximizeLayout: null });
  const captured = capture
    ? (useDockviewStore.getState().captureCurrentLayout() as LayoutState)
    : fromDockviewApi(editor);
  const source = JSON.parse(JSON.stringify(captured)) as LayoutState;
  expect(validateReusableLayout(source).valid).toBe(true);
  clearAllPinnedTargets();
  return { source, ...geometry };
}
/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "absent pinned root lifecycle through %s enforcement",
  (boundary) => {
    it.each(["preserve", "edit", "released-control"] as const)(
      "keeps the intended geometry after hidden pinned-last %s",
      async (mode) => {
        const source = sourceFromSettings([400, 1000, 200], true);
        const [center, right, plugin] = source.columns;
        right.groups[0].panels.push({ id: "changes", component: "changes", title: "Changes" });
        source.columns = [center, plugin, right];
        expect(validateReusableLayout(source).valid).toBe(true);
        const immutable = structuredClone(source);
        const api = await build(source, boundary);
        expectGeometry(api, [320, 160, 320]);
        useDockviewStore.getState().toggleRightPanels();
        await settle();
        expect(api.getPanel("files")).toBeUndefined();
        expect(useDockviewStore.getState().rightPanelsVisible).toBe(false);
        if (mode !== "preserve") {
          api.addPanel({ id: "todos", component: "todos", position: { referencePanel: "chat" } });
          expect(splitPanel(api, "todos", "right")).toBe(true);
          resize(api, 800, boundary);
          await settle();
          expect(
            mergeGroup(api, api.getPanel("todos")!.group.id, api.getPanel(PLUGIN.id)!.group.id),
          ).toBe(true);
          api.getPanel("todos")!.api.close();
          resize(api, 800, boundary);
          await settle();
          expectGeometry(api, [400, 400]);
        }
        if (mode === "released-control") clearReusableRootGeometry(ENV);
        if (mode === "preserve") {
          resize(api, 1600, boundary);
          await settle();
        }
        useDockviewStore.getState().toggleRightPanels();
        await settle();
        resize(api, 1600, boundary);
        await settle();
        expectGeometry(api, mode === "preserve" ? [400, 200, 1000] : [480, 120, 1000]);
        expect(source).toEqual(immutable);
        expect(getManualRightWidth(ENV)).toBeNull();
      },
    );
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "distinct settings tool roots through %s enforcement",
  (boundary) => {
    it.each(
      [false, true].flatMap((capture) =>
        (Object.keys(TOOL_ROOT_CASES) as Array<keyof typeof TOOL_ROOT_CASES>).map((split) => ({
          capture,
          split,
        })),
      ),
    )("round-trips $split with reusable capture $capture", async ({ split, capture }) => {
      const { source, wide, narrow, pinnedPanel, flexiblePanel } = sourceFromToolSplit(
        split,
        capture,
      );
      const immutable = structuredClone(source);
      const api = await build(source, boundary);
      for (const width of [1600, 800, 1600, 800, 1600]) {
        resize(api, width, boundary);
        await settle();
        expectGeometry(api, width === 800 ? narrow : wide);
        expect(api.getPanel(pinnedPanel)!.group.maximumWidth).toBe(width === 800 ? 320 : 1120);
        expect(api.getPanel(flexiblePanel)!.group.maximumWidth).toBeGreaterThanOrEqual(wide[2]);
      }
      const restored = fromDockviewApi(api);
      expect(new Set(restored.columns.map((column) => column.id)).size).toBe(wide.length);
      expect(
        restored.columns.filter((column) => column.pinned && column.id !== "sidebar"),
      ).toHaveLength(1);
      expect(
        restored.columns
          .find((column) => column.id === "right")!
          .groups.some((group) => group.panels.some((panel) => panel.id === pinnedPanel)),
      ).toBe(true);
      expect(source).toEqual(immutable);
      expect(getManualRightWidth(ENV)).toBeNull();
      if (split === "changes") {
        setManualRightWidth(ENV, 700);
        resize(api, 1600, boundary);
        await settle();
        expect(api.getPanel("files")!.group.width).toBe(700);
        expect(getManualRightWidth(ENV)).toBe(700);
        expect(api.getPanel("changes")!.group.maximumWidth).toBeGreaterThanOrEqual(400);
      }
    });
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-001.10, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "saved and vertical tool roots through %s enforcement",
  (boundary) => {
    it("applies valid JSON saved before tool-root IDs were disambiguated", async () => {
      const { source } = sourceFromToolSplit("changes", true);
      source.columns[2].id = "right";
      source.columns[2].pinned = true;
      const saved = JSON.parse(JSON.stringify(source)) as LayoutState;
      const immutable = structuredClone(saved);
      const validation = validateReusableLayout(saved);
      expect(validation.valid).toBe(true);
      if (!validation.valid) throw new Error("Expected valid saved tool roots");
      const api = await build(validation.layout, boundary);
      for (const width of [1600, 800, 1600]) {
        resize(api, width, boundary);
        await settle();
        expectGeometry(api, width === 800 ? [240, 320, 240] : [400, 800, 400]);
      }
      expect(saved).toEqual(immutable);
      expect(getManualRightWidth(ENV)).toBeNull();
    });

    it("keeps separate tool rows outside horizontal width pinning", async () => {
      const { source } = sourceFromToolSplit("changes", true);
      source.rootOrientation = "VERTICAL";
      const immutable = structuredClone(source);
      const api = await build(source, boundary);
      for (const width of [800, 1600, 800]) {
        resize(api, width, boundary);
        await settle();
        expectGeometry(api, [175, 350, 175]);
        expect(api.getPanel("files")!.group.maximumWidth).toBeGreaterThanOrEqual(1600);
        expect(api.getPanel("changes")!.group.maximumWidth).toBeGreaterThanOrEqual(1600);
      }
      expect(getManualRightWidth(ENV)).toBeNull();
      expect(source).toEqual(immutable);
    });
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.3, AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)(
  "tool-root constraint ownership through %s enforcement",
  (boundary) => {
    it.each(["none", "at-transfer", "while-owned"] as const)(
      "releases transferred host bounds without clearing component bounds %s",
      async (componentBounds) => {
        const source = sourceFromSettings([400, 1000, 200], true);
        source.columns[1].groups[0].panels.push({
          id: "changes",
          component: "changes",
          title: "Changes",
        });
        const api = await build(source, boundary);
        if (boundary === "state") stops.push(setupSashDragCapToggle(api));
        expect(api.getPanel("files")!.group.maximumWidth).toBe(320);
        if (componentBounds !== "none")
          api.getPanel("files")!.group.api.setConstraints({ maximumWidth: 700, minimumWidth: 200 });
        if (componentBounds === "while-owned") {
          resize(api, 800, boundary);
          await settle();
          expect(api.getPanel("files")!.group.maximumWidth).toBe(320);
        }
        expect(splitPanel(api, "changes", "left")).toBe(true);
        resize(api, 2400, boundary);
        await settle();
        expect(api.getPanel("changes")!.group.width).toBe(1500);
        const sv = getRootSplitview(api);
        const sash: HTMLElement = sv.sashes[2].container;
        sash.dispatchEvent(new MouseEvent("mousedown", { bubbles: true, button: 0 }));
        sv.resizeView(2, 500);
        expect(api.getPanel("files")!.group.width).toBe(500);
        sash.dispatchEvent(new MouseEvent("mouseup", { bubbles: true, button: 0 }));
        await settle();
        api.getPanel("chat")!.api.setTitle("Agent after tool split resize");
        await settle();
        expect(getManualRightWidth(ENV)).toBeNull();
        if (componentBounds !== "none") {
          expect(api.getPanel("files")!.group.maximumWidth).toBe(700);
          expect(api.getPanel("files")!.group.minimumWidth).toBe(200);
        }
      },
    );

    it("keeps the existing first-tool manual owner when Files moves to a later root", async () => {
      const source = sourceFromSettings([400, 1000, 200], true);
      source.columns[1].groups[0].panels.push({
        id: "changes",
        component: "changes",
        title: "Changes",
      });
      const api = await build(source, boundary);
      setManualRightWidth(ENV, 700);
      resize(api, 1600, boundary);
      await settle();
      const originalGroup = api.getPanel("files")!.group;
      expect(originalGroup.width).toBe(700);
      expect(splitPanel(api, "files", "right")).toBe(true);
      resize(api, 1600, boundary);
      await settle();
      expect(originalGroup.width).toBe(700);
      expect(api.getPanel("changes")!.group).toBe(originalGroup);
      expect(getManualRightWidth(ENV)).toBe(700);
    });
  },
);

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)("raw explicit profiles through %s", (boundary) => {
  it("applies raw saved JSON without aliasing roots", async () => {
    const { source } = sourceFromToolSplit("changes", true);
    source.columns[2].id = "right";
    source.columns[2].pinned = true;
    const immutable = structuredClone(source);
    const api = await build(sourceFromSettings([1200, 400], true), boundary);
    useDockviewStore.getState().applyCustomLayout(
      {
        id: "raw-custom",
        name: "Raw tool roots",
        isDefault: false,
        createdAt: "2026-10-24T00:00:00Z",
        layout: source as unknown as Record<string, unknown>,
      },
      { envId: ENV, activeSessionId: null, sessionIds: [] },
    );
    await settle();
    expectGeometry(api, [240, 320, 240]);
    for (const width of [1600, 800, 1600]) {
      resize(api, width, boundary);
      await settle();
      expectGeometry(api, width === 800 ? [240, 320, 240] : [400, 800, 400]);
    }
    expect(source).toEqual(immutable);
    expect(getManualRightWidth(ENV)).toBeNull();
  });
});

/** @covers AC-UI-TASK-LAYOUT-PROFILES-002.6 */
describe.each(["mounted", "state"] as const)("ordinary no-right edits through %s", (boundary) => {
  it.each([false, true])(
    "never revives source after move/undo with explicit release %s",
    async (release) => {
      const source = sourceFromSettings([400, 1000, 200], true);
      const immutable = structuredClone(source);
      const api = await build(source, "state");
      stops.push(setupGroupTracking(api));
      if (boundary === "mounted") stops.push(setupSashDragCapToggle(api));
      expect(movePanelToGroup(api, "files", api.getPanel("chat")!.group.id)).toBe(true);
      await settle();
      expect(useDockviewStore.getState().hiddenRightPane).toBeNull();
      expect(useDockviewStore.getState().rightPanelsVisible).toBe(false);
      resize(api, 800, boundary);
      await settle();
      expectGeometry(api, [400, 400]);
      if (release) clearReusableRootGeometry(ENV);
      expect(splitPanel(api, "files", "right")).toBe(true);
      await settle();
      resize(api, 1600, boundary);
      await settle();
      expectGeometry(api, [500, 1000, 100]);
      expect(source).toEqual(immutable);
      expect(getManualRightWidth(ENV)).toBeNull();
    },
  );
});
