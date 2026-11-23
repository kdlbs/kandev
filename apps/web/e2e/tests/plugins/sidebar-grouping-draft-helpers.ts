import { createHash } from "node:crypto";
import { expect, type Locator, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import { SidebarFilterPopoverPage } from "../../pages/sidebar-filter-popover";
import {
  SIDEBAR_GROUPING,
  groupingValueKey,
  saveGroupingView,
  writeGroupingState,
} from "./sidebar-plugin-grouping-helpers";

type Seed = { workspaceId: string; workflowId: string; startStepId: string };
type DraftScenario = {
  page: Page;
  api: ApiClient;
  seed: Seed;
  mobile: boolean;
  finish: "discard" | "save as" | "overwrite";
  info: TestInfo;
};

async function activate(control: Locator, mobile: boolean) {
  await expect(control).toHaveCount(1);
  if (mobile) await control.tap();
  else await control.click();
}

async function openSurface(page: Page, workspaceId: string, mobile: boolean, taskId: string) {
  if (mobile) await page.setViewportSize({ width: 320, height: 700 });
  await page.goto(mobile ? `/t/${taskId}` : `/?workspaceId=${workspaceId}`);
  if (mobile) await page.getByTestId("mobile-task-picker-trigger").tap();
  return mobile
    ? page.getByRole("dialog", { name: "Tasks", exact: true })
    : page.getByTestId("task-sidebar");
}

async function openEditor(page: Page, mobile: boolean) {
  const editor = new SidebarFilterPopoverPage(page);
  if (mobile) {
    await page
      .getByRole("dialog", { name: "Tasks", exact: true })
      .getByRole("button", { name: "Sidebar filters", exact: true })
      .tap();
    await expect(editor.popover).toBeVisible();
  } else await editor.open();
  return editor;
}

export async function exerciseChangedGroupingDraft(scenario: DraftScenario) {
  const { page, api, seed, mobile, finish, info } = scenario;
  const baseline = (await api.getUserSettings()).settings.sidebar_views_by_workspace[
    seed.workspaceId
  ];
  try {
    const root = await api.createTask(seed.workspaceId, "Draft disclosure root", {
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
    });
    await writeGroupingState(api, seed.workspaceId, {
      snapshot: {
        version: 1,
        revision: "draft",
        complete: true,
        groups: [{ id: "keep", label: "Keep" }],
        memberships: [{ task_id: root.id, group_id: "keep" }],
      },
    });
    await saveGroupingView(api, seed.workspaceId);
    const state = (await api.getUserSettings()).settings.sidebar_views_by_workspace[
      seed.workspaceId
    ];
    const base = state.views.find((view) => view.id === state.active_view_id)!;
    const baseReference = "plugin:kandev-plugin-e2e:fixture-option-01";
    const baseKey =
      "plugin-group:" +
      createHash("sha256")
        .update(baseReference + "\0keep")
        .digest("hex");
    const key = groupingValueKey("keep");
    await api.saveUserSettings({
      sidebar_view_state: {
        workspace_id: seed.workspaceId,
        views_upsert: [{ ...base, group: baseReference, collapsed_groups: [baseKey] }],
      },
    });
    let surface = await openSurface(page, seed.workspaceId, mobile, root.id);
    await expect(
      surface.locator(`[data-testid="sidebar-group-header"][data-group-key="${baseKey}"]`),
    ).toHaveAttribute("aria-expanded", "false");
    let editor = await openEditor(page, mobile);
    if (
      (await editor.popover
        .getByTestId("sidebar-group-settings-toggle")
        .getAttribute("aria-expanded")) !== "true"
    ) {
      await activate(editor.popover.getByTestId("sidebar-group-settings-toggle"), mobile);
    }
    await activate(editor.popover.getByTestId("group-key-select"), mobile);
    await activate(
      page.getByRole("listbox").getByRole("option", { name: "Fixture grouping", exact: true }),
      mobile,
    );
    await editor.close();
    await expect(surface).toBeVisible();
    let header = surface.locator(`[data-testid="sidebar-group-header"][data-group-key="${key}"]`);
    await expect(header).toHaveAttribute("aria-expanded", "true");
    await activate(header, mobile);
    await expect(header).toHaveAttribute("aria-expanded", "false");
    const preferences = async () =>
      (await api.getUserSettings()).settings.sidebar_views_by_workspace[seed.workspaceId];
    await expect.poll(async () => (await preferences()).draft?.collapsed_groups).toEqual([key]);
    expect(
      (await preferences()).views.find((view) => view.id === base.id)?.collapsed_groups,
    ).toEqual([baseKey]);
    surface = await openSurface(page, seed.workspaceId, mobile, root.id);
    header = surface.locator(`[data-testid="sidebar-group-header"][data-group-key="${key}"]`);
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await activate(header, mobile);
    await expect(header).toHaveAttribute("aria-expanded", "true");
    await activate(header, mobile);
    await expect(header).toHaveAttribute("aria-expanded", "false");
    await page.screenshot({
      path: info.outputPath(`draft-disclosure-${finish}.png`),
      animations: "disabled",
    });
    editor = await openEditor(page, mobile);
    if (finish === "save as") {
      await activate(editor.popover.getByTestId("view-save-as-button"), mobile);
      await editor.popover.getByTestId("view-save-as-name-input").fill("Independent draft copy");
      await activate(editor.popover.getByTestId("view-save-as-confirm"), mobile);
    } else
      await activate(
        editor.popover.getByTestId(
          finish === "discard" ? "view-discard-button" : "view-save-button",
        ),
        mobile,
      );
    await editor.close();
    await expect.poll(async () => (await preferences()).draft).toBeNull();
    const saved = await preferences();
    const active = saved.views.find((view) => view.id === saved.active_view_id)!;
    expect(active.group).toBe(finish === "discard" ? baseReference : SIDEBAR_GROUPING);
    expect(active.collapsed_groups).toEqual([finish === "discard" ? baseKey : key]);
    if (finish !== "overwrite")
      expect(saved.views.find((view) => view.id === base.id)?.collapsed_groups).toEqual([baseKey]);
    surface = await openSurface(page, seed.workspaceId, mobile, root.id);
    await expect(
      surface.locator(
        `[data-testid="sidebar-group-header"][data-group-key="${finish === "discard" ? baseKey : key}"]`,
      ),
    ).toHaveAttribute("aria-expanded", "false");
    await page.screenshot({
      path: info.outputPath(`saved-disclosure-${finish}.png`),
      animations: "disabled",
    });
  } finally {
    if (baseline)
      await api.saveUserSettings({
        sidebar_view_state: { workspace_id: seed.workspaceId, ...baseline },
      });
  }
}
