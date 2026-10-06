import { expect, test } from "../../fixtures/test-base";
import { expectControlHeight } from "../../helpers/control-sizing";
import { waitForFiniteAnimations } from "../../helpers/animations";
import type { Locator } from "@playwright/test";
import fs from "node:fs";
import path from "node:path";
import type { BackendContext } from "../../fixtures/backend";

const HIDDEN_DIRECTORY = ".hidden-project";
const VISIBLE_DIRECTORY = "visible-project";
const TOUCH_TARGET_PX = 44;

// The e2e backend runs with HOME set to its temporary root, so the browser
// lists that root and the fixture stays out of the developer's real home.
function createBrowsableDirectories(backend: BackendContext): void {
  fs.mkdirSync(path.join(backend.tmpDir, HIDDEN_DIRECTORY), { recursive: true });
  fs.mkdirSync(path.join(backend.tmpDir, VISIBLE_DIRECTORY), { recursive: true });
}

// A path deep enough that its breadcrumb cannot fit the popover, which is what
// makes the trailing-edge pin load-bearing rather than incidental.
const DEEP_SEGMENTS = [
  "alpha-directory",
  "bravo-directory",
  "charlie-directory",
  "delta-directory",
  "echo-directory",
  "foxtrot-directory",
  "golf-directory",
];

function createDeepDirectory(backend: BackendContext): void {
  fs.mkdirSync(path.join(backend.tmpDir, ...DEEP_SEGMENTS), { recursive: true });
}

async function navigateIntoDeepDirectory(picker: Locator): Promise<void> {
  for (const segment of DEEP_SEGMENTS) {
    await picker.getByRole("button", { name: segment, exact: true }).tap();
  }
}

// @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.10, AC-WORKSPACES-HIDDEN-FOLDERS-001.3
test("coarse pointer grows the reveal control and keeps the reveal usable by touch", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  test.setTimeout(120_000);
  createBrowsableDirectories(backend);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Reveal hidden folders on a phone",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).primary_executor_type, {
      timeout: 30_000,
    })
    .toBeTruthy();

  await testPage.goto(`/t/${task.id}`);
  // On a phone the Files surface is a drawer opened from its own button, and the
  // add-sources surface is a drawer rather than a dialog.
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  const entryPoint = testPage.getByTestId("files-workspace-actions");
  await expect(entryPoint).toBeVisible();
  await entryPoint.tap();
  await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).tap();
  const drawer = testPage.getByTestId("add-workspace-sources-drawer");
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "Add folder" }).tap();
  await expect(drawer.getByTestId("workspace-source-row")).toBeVisible();

  await drawer.getByTestId("folder-picker-trigger").last().tap();
  const picker = testPage
    .locator('[data-testid="folder-picker-popover"][data-state="open"]')
    .last();
  await expect(picker).toBeVisible();
  const control = picker.getByTestId("directory-browser-show-hidden");
  const toggle = picker.getByRole("switch", { name: "Hidden folders" });

  // The label wraps the switch, so the whole band is the tap target; it must meet
  // the coarse-pointer minimum and must not push the entry list out of reach.
  await expectControlHeight(control, TOUCH_TARGET_PX, 1);
  await expect(picker.getByTestId("folder-picker-entry").first()).toBeVisible();

  await control.tap();
  await expect(toggle).toHaveAttribute("aria-checked", "true");
  const entries = picker.getByTestId("folder-picker-entry");
  await expect(entries.filter({ hasText: HIDDEN_DIRECTORY })).toHaveCount(1);
  await expect(entries.filter({ hasText: VISIBLE_DIRECTORY })).toHaveCount(1);
  await prCapture.screenshot("directory-browser-hidden-phone", { fullPage: false });

  // The revealed directory stays selectable with a touch gesture.
  await entries.filter({ hasText: HIDDEN_DIRECTORY }).tap();
  const choose = picker.getByTestId("folder-picker-choose");
  await expect(choose).toBeEnabled();
  await choose.tap();
  await expect(testPage.getByTestId("folder-picker-trigger").last()).toContainText(
    HIDDEN_DIRECTORY,
  );
});

// @covers AC-WORKSPACES-HIDDEN-FOLDERS-001.10
test("a breadcrumb too long to fit keeps the reveal control inside the popover", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  test.setTimeout(120_000);
  createBrowsableDirectories(backend);
  createDeepDirectory(backend);
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Reveal stays pinned on a long path",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
      executor_profile_id: seedData.worktreeExecutorProfileId,
    },
  );
  await expect
    .poll(async () => (await apiClient.getTask(task.id)).primary_executor_type, {
      timeout: 30_000,
    })
    .toBeTruthy();

  await testPage.goto(`/t/${task.id}`);
  await testPage.getByRole("button", { name: "Files", exact: true }).tap();
  const entryPoint = testPage.getByTestId("files-workspace-actions");
  await entryPoint.tap();
  await testPage.getByRole("menuitem", { name: "Add Repositories to workspace" }).tap();
  const drawer = testPage.getByTestId("add-workspace-sources-drawer");
  await expect(drawer).toBeVisible();
  await drawer.getByRole("button", { name: "Add folder" }).tap();
  await drawer.getByTestId("folder-picker-trigger").last().tap();
  const picker = testPage
    .locator('[data-testid="folder-picker-popover"][data-state="open"]')
    .last();
  await expect(picker).toBeVisible();

  const control = picker.getByTestId("directory-browser-show-hidden");
  const measure = async () => {
    const controlBox = await control.boundingBox();
    const popoverBox = await picker.boundingBox();
    if (!controlBox || !popoverBox) throw new Error("reveal control has no layout box");
    return {
      // Distance from the popover's trailing edge. A pinned control is flush, so
      // this stays zero while the breadcrumb grows and scrolls behind it.
      trailingGap: popoverBox.x + popoverBox.width - (controlBox.x + controlBox.width),
      inside: controlBox.x >= popoverBox.x,
      // The breadcrumb is the control's scrollable sibling.
      breadcrumbScrolls: await control.evaluate((element) => {
        const breadcrumb = element.previousElementSibling as HTMLElement | null;
        return breadcrumb ? breadcrumb.scrollWidth > breadcrumb.clientWidth : false;
      }),
      documentOverflow: await testPage.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      ),
    };
  };

  // Measure only settled geometry: an animating popover reports a narrower box
  // and a scaled control, which would make the comparison meaningless.
  await waitForFiniteAnimations(picker);
  const before = await measure();
  await navigateIntoDeepDirectory(picker);
  await waitForFiniteAnimations(picker);
  const after = await measure();

  expect(after.inside).toBe(true);
  // Flush against the popover's trailing edge both before and after the long
  // path, so a deeper path can never push the reveal control out of reach.
  expect(before.trailingGap).toBeCloseTo(0, 0);
  expect(after.trailingGap).toBeCloseTo(0, 0);
  // The long path really does overflow, which is what makes the pin load-bearing.
  expect(after.breadcrumbScrolls).toBe(true);
  expect(after.documentOverflow).toBeLessThanOrEqual(0);
  await expect(control).toBeVisible();
});
