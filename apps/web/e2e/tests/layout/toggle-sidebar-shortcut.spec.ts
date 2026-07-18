import { test, expect } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";
import path from "node:path";
import type { SeedData } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

// The TOGGLE_SIDEBAR shortcut lives on a global app-root listener (useAppShortcuts),
// so Cmd/Ctrl+B must collapse/expand the unified AppSidebar on every route — not
// just inside the dockview session editor. It is UNBOUND by default on this
// branch, so each test binds it to Cmd/Ctrl+B first (mirroring how a user would
// set it in Settings).

const MODIFIER = process.platform === "darwin" ? "Meta" : "Control";

/** Bind TOGGLE_SIDEBAR to Cmd/Ctrl+B (unbound by default) for the active user. */
async function bindToggleSidebar(apiClient: ApiClient, seedData: SeedData): Promise<void> {
  await apiClient.saveUserSettings({
    workspace_id: seedData.workspaceId,
    keyboard_shortcuts: {
      TOGGLE_SIDEBAR: { key: "b", modifiers: { ctrlOrCmd: true } },
    },
  });
}

/** Press Cmd/Ctrl+B and assert the AppSidebar's collapsed state flips both ways. */
async function expectShortcutTogglesSidebar(page: Page, sidebar: Locator): Promise<void> {
  await expect(sidebar).toBeVisible();
  // The shortcut is intentionally ignored while typing — clear focus first so a
  // page-autofocused input can't suppress it.
  await page.evaluate(() => (document.activeElement as HTMLElement | null)?.blur());

  const initial = (await sidebar.getAttribute("data-collapsed")) ?? "false";
  const flipped = initial === "true" ? "false" : "true";

  await page.keyboard.press(`${MODIFIER}+b`);
  await expect(sidebar).toHaveAttribute("data-collapsed", flipped);

  await page.keyboard.press(`${MODIFIER}+b`);
  await expect(sidebar).toHaveAttribute("data-collapsed", initial);
}

type SidebarToggleFrame = {
  layoutWidth: number;
  panelWidth: number;
  rightPaneLeft: number;
  rightPaneWidth: number;
  contentLeft: number;
  contentWidth: number;
};

type SidebarToggleCapture = {
  frames: SidebarToggleFrame[];
  widthTransitionDurationMs: number;
  transitionProperties: string[];
};

type RightPaneTarget = {
  panelTestId: "files-panel" | "changes-panel";
  contentSelector?: string;
};

/** Capture painted layout frames while the visible sidebar toggle changes state. */
async function captureSidebarToggleFrames(
  page: Page,
  buttonLabel: "Collapse sidebar" | "Expand sidebar",
  target: RightPaneTarget,
): Promise<SidebarToggleCapture> {
  return page.evaluate(
    async ({ label, paneTarget }) => {
      const panel = document.querySelector('[data-testid="app-sidebar"]') as HTMLElement | null;
      const layout =
        (document.querySelector('[data-testid="app-sidebar-layout"]') as HTMLElement | null) ??
        panel;
      const rightPane = Array.from(
        document.querySelectorAll<HTMLElement>(`[data-testid="${paneTarget.panelTestId}"]`),
      ).find((candidate) => candidate.getBoundingClientRect().width > 0);
      const content = paneTarget.contentSelector
        ? rightPane?.querySelector<HTMLElement>(paneTarget.contentSelector)
        : rightPane;
      const button = panel?.querySelector<HTMLButtonElement>(`[aria-label="${label}"]`);
      if (!layout || !panel || !rightPane || !content || !button) {
        throw new Error(
          `Missing sidebar, visible ${paneTarget.panelTestId} content, or ${label} button`,
        );
      }

      const readFrame = (): SidebarToggleFrame => {
        const layoutBox = layout.getBoundingClientRect();
        const panelBox = panel.getBoundingClientRect();
        const rightPaneBox = rightPane.getBoundingClientRect();
        const contentBox = content.getBoundingClientRect();
        return {
          layoutWidth: layoutBox.width,
          panelWidth: panelBox.width,
          rightPaneLeft: rightPaneBox.left,
          rightPaneWidth: rightPaneBox.width,
          contentLeft: contentBox.left,
          contentWidth: contentBox.width,
        };
      };

      const panelStyle = getComputedStyle(panel);
      const transitionProperties = panelStyle.transitionProperty
        .split(",")
        .map((property) => property.trim());
      const transitionDurationsMs = panelStyle.transitionDuration.split(",").map((duration) => {
        const value = Number.parseFloat(duration);
        return duration.trim().endsWith("ms") ? value : value * 1000;
      });
      const widthTransitionIndex = transitionProperties.indexOf("width");
      const widthTransitionDurationMs =
        widthTransitionIndex < 0
          ? 0
          : (transitionDurationsMs[widthTransitionIndex % transitionDurationsMs.length] ?? 0);
      const frames = [readFrame()];
      button.click();
      const sampleUntil = performance.now() + 400;
      do {
        // rAF callbacks run before ResizeObserver delivery and paint. Hop through
        // a timer so each measurement reflects geometry the user actually saw.
        await new Promise<void>((resolve) => {
          requestAnimationFrame(() => setTimeout(resolve, 0));
        });
        frames.push(readFrame());
      } while (performance.now() < sampleUntil);
      return { frames, widthTransitionDurationMs, transitionProperties };
    },
    { label: buttonLabel, paneTarget: target },
  );
}

function expectStableAnimatedToggle(capture: SidebarToggleCapture): void {
  const { frames } = capture;
  const changedFrames = (read: (frame: SidebarToggleFrame) => number): number[] => {
    const changes: number[] = [];
    for (let index = 1; index < frames.length; index += 1) {
      if (Math.abs(read(frames[index]) - read(frames[index - 1])) > 1) changes.push(index);
    }
    return changes;
  };

  const expectStable = (label: string, read: (frame: SidebarToggleFrame) => number): void => {
    const values = frames.map(read);
    const span = Math.max(...values) - Math.min(...values);
    expect(span, `${label} moved ${span.toFixed(2)}px across painted frames`).toBeLessThanOrEqual(
      1,
    );
  };

  expectStable("Right pane position", (frame) => frame.rightPaneLeft);
  expectStable("Right pane width", (frame) => frame.rightPaneWidth);
  expectStable("Right pane content position", (frame) => frame.contentLeft);
  expectStable("Right pane content width", (frame) => frame.contentWidth);

  expect(capture.transitionProperties).toContain("width");
  expect(capture.widthTransitionDurationMs).toBeGreaterThanOrEqual(290);
  expect(capture.widthTransitionDurationMs).toBeLessThanOrEqual(310);

  const layoutChanges = changedFrames((frame) => frame.layoutWidth);
  expect(
    layoutChanges,
    `Sidebar layout width changed on frames ${layoutChanges.join(", ")}`,
  ).toHaveLength(1);

  const panelChanges = changedFrames((frame) => frame.panelWidth);
  const panelWidths = frames.map((frame) => frame.panelWidth);
  const minimumPanelWidth = Math.min(...panelWidths);
  const maximumPanelWidth = Math.max(...panelWidths);
  expect(
    maximumPanelWidth - minimumPanelWidth,
    "Visual sidebar should travel between its expanded and collapsed widths",
  ).toBeGreaterThan(200);
  expect(
    panelWidths
      .slice(1, -1)
      .some((width) => width > minimumPanelWidth + 1 && width < maximumPanelWidth - 1),
    `Visual sidebar never painted an intermediate width: ${panelWidths.join(", ")}`,
  ).toBe(true);
  expect(
    panelChanges.at(-1) ?? 0,
    "Visual sidebar should keep animating after the layout shell settles",
  ).toBeGreaterThan(layoutChanges[0]);
  expect(Math.abs(frames.at(-1)!.panelWidth - frames.at(-1)!.layoutWidth)).toBeLessThanOrEqual(1);
}

async function expectStablePaneDuringSidebarToggle(
  page: Page,
  target: RightPaneTarget,
): Promise<void> {
  const collapseCapture = await captureSidebarToggleFrames(page, "Collapse sidebar", target);
  expectStableAnimatedToggle(collapseCapture);

  const expandCapture = await captureSidebarToggleFrames(page, "Expand sidebar", target);
  expectStableAnimatedToggle(expandCapture);
}

test.describe("Toggle sidebar shortcut (global)", () => {
  test("toggles the AppSidebar on the Kanban board", async ({ testPage, apiClient, seedData }) => {
    await bindToggleSidebar(apiClient, seedData);
    await testPage.goto("/");
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));
  });

  test("toggles the AppSidebar on a Settings page", async ({ testPage, apiClient, seedData }) => {
    await bindToggleSidebar(apiClient, seedData);
    await testPage.goto("/settings/general");
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));
  });

  test("animates the AppSidebar without moving Files or Changes", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    await testPage.setViewportSize({ width: 1600, height: 900 });
    await bindToggleSidebar(apiClient, seedData);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Toggle sidebar shortcut session",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();

    // Target the global rail (app-sidebar), not the dockview's per-session
    // sidebar (task-sidebar).
    await expectShortcutTogglesSidebar(testPage, testPage.getByTestId("app-sidebar"));

    await session.waitForDockviewReady();
    await session.clickTab("Files");
    await expect(session.files).toBeVisible();
    await expect(session.fileTreeNode("walkthrough_base.txt")).toBeVisible();

    await expectStablePaneDuringSidebarToggle(testPage, {
      panelTestId: "files-panel",
      contentSelector: '[data-testid="file-tree-node"][data-path="walkthrough_base.txt"]',
    });

    const git = new GitHelper(
      path.join(backend.tmpDir, "repos", "e2e-repo"),
      makeGitEnv(backend.tmpDir),
    );
    try {
      git.modifyFile("walkthrough_base.txt", "changed while checking sidebar stability\n");

      await session.clickTab("Changes");
      await expect(session.changes).toBeVisible();
      await expect(session.changesFileRow("walkthrough_base.txt")).toBeVisible({ timeout: 15_000 });
      await expectStablePaneDuringSidebarToggle(testPage, {
        panelTestId: "changes-panel",
        contentSelector: '[data-changes-file="walkthrough_base.txt"]',
      });
    } finally {
      git.exec('git checkout -- "walkthrough_base.txt"');
    }
  });
});
