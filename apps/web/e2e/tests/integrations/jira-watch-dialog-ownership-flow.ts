import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import { JiraSettingsPage } from "../../pages/jira-settings-page";
import { waitForHttp } from "../../helpers/causal-waits";
import type { JiraIssueWatch } from "../../../lib/types/jira";
import { apiJson, type JiraWatchFixture } from "./jira-watch-dialog-ownership-fixture";

const endpoint = "/api/v1/jira/watches/issue";
const rawOld = "  project = OWN ORDER BY created DESC  ";
const rawNew = "  project = OWN AND status = Open  ";
async function press(control: Locator, mobile: boolean) {
  await expect(control).toHaveCount(1);
  if (mobile) await control.tap();
  else await control.click();
}
async function activateWorkspace(page: Page, workspaceId: string, mobile: boolean) {
  await page.goto("/");
  if (mobile) {
    await press(page.getByTestId("app-nav-trigger"), mobile);
    await press(
      page.getByRole("dialog", { name: "Menu" }).getByTestId("mobile-workspace-trigger"),
      mobile,
    );
    await press(page.getByTestId(`mobile-workspace-item-${workspaceId}`), mobile);
  } else {
    await press(page.getByTestId("sidebar-workspace-trigger"), mobile);
    await press(page.getByTestId(`sidebar-workspace-item-${workspaceId}`), mobile);
  }
}
function watchRow(page: Page, id: string, mobile: boolean) {
  return page.getByTestId(`jira-watch-${mobile ? "mobile-row" : "row"}-${id}`);
}
async function choose(page: Page, label: string, option: string, mobile: boolean) {
  const trigger = page
    .getByRole("dialog")
    .getByText(label, { exact: true })
    .locator("xpath=..")
    .getByRole("combobox");
  await press(trigger, mobile);
  await press(page.getByRole("listbox").getByRole("option", { name: option, exact: true }), mobile);
}
async function openCreate(page: Page, fixture: JiraWatchFixture, mobile: boolean) {
  await press(page.getByRole("button", { name: "New watcher", exact: true }), mobile);
  const dialog = page.getByRole("dialog");
  await choose(page, "Workspace", fixture.name, mobile);
  await choose(page, "Workflow", fixture.name, mobile);
  await choose(page, "Workflow Step", `${fixture.name} step`, mobile);
  await choose(page, "Agent Profile", fixture.agentLabel, mobile);
  await choose(page, "Executor Profile", fixture.profileName, mobile);
  // Keep the disposable watch paused; this suite never launches provider tasks.
  await press(dialog.getByRole("switch"), mobile);
}
async function readWatches(api: ApiClient, workspaceId: string) {
  return (
    await apiJson<{ watches: JiraIssueWatch[] }>(
      api,
      "GET",
      `${endpoint}?workspace_id=${workspaceId}`,
    )
  ).watches;
}

async function holdWrite(page: Page, method: string, savePath: string, workspaceId: string) {
  const admitted = Promise.withResolvers<JiraIssueWatch>();
  const release = Promise.withResolvers<void>();
  const finished = Promise.withResolvers<void>();
  const writes: Record<string, unknown>[] = [];
  let held = false;
  let routeError: unknown;
  const handler = async (route: Route) => {
    if (route.request().method() !== method) return route.continue();
    writes.push(route.request().postDataJSON());
    if (held) return route.continue();
    held = true;
    try {
      const response = await route.fetch();
      expect(response.ok(), await response.text()).toBe(true);
      expect(new URL(route.request().url()).pathname).toBe(savePath);
      expect(new URL(route.request().url()).searchParams.get("workspace_id")).toBe(
        method === "POST" ? null : workspaceId,
      );
      admitted.resolve((await response.json()) as JiraIssueWatch);
      await release.promise;
      await route.fulfill({ response });
    } catch (error) {
      routeError = error;
      admitted.reject(error);
      await route.abort();
    } finally {
      finished.resolve();
    }
  };
  const pattern = `**${savePath}*`;
  await page.route(pattern, handler);
  return {
    admitted: admitted.promise,
    writes,
    release: async () => {
      release.resolve();
      await finished.promise;
      if (routeError) throw routeError;
    },
    cleanup: async () => {
      release.resolve();
      if (held) await finished.promise;
      await page.unroute(pattern, handler);
      if (routeError) throw routeError;
    },
  };
}

async function captureDraft(page: Page, mobile: boolean, create: boolean) {
  await page.screenshot({
    path: process.env.KANDEV_JIRA_WATCH_SCREENSHOTS
      ? `${process.env.KANDEV_JIRA_WATCH_SCREENSHOTS}/${mobile ? "phone" : "desktop"}-${create ? "create" : "edit"}.png`
      : undefined,
  });
}

// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.1, AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.2
// @covers AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.3, AC-INTEGRATIONS-JIRA-WATCH-DIALOG-001.6
export async function jiraWatchDialogFlow(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  fixture: JiraWatchFixture,
  { mobile, create }: { mobile: boolean; create: boolean },
) {
  let original: JiraIssueWatch | undefined;
  if (!create)
    original = await apiJson<JiraIssueWatch>(
      api,
      "POST",
      `${endpoint}?workspace_id=${fixture.workspaceId}`,
      {
        workspaceId: fixture.workspaceId,
        workflowId: fixture.workflowId,
        workflowStepId: fixture.stepId,
        repositoryId: fixture.repositoryId,
        baseBranch: "main",
        agentProfileId: seed.agentProfileId,
        executorProfileId: fixture.profileId,
        jql: "project = OWN",
        prompt: "Investigate {{issue.title}}",
        enabled: false,
        pollIntervalSeconds: 300,
        maxInflightTasks: 5,
      },
    );
  await activateWorkspace(page, fixture.workspaceId, mobile);
  await new JiraSettingsPage(page).gotoWorkspace(fixture.workspaceId);
  const open = async () => {
    if (create) await openCreate(page, fixture, mobile);
    else {
      const row = watchRow(page, original!.id, mobile);
      await press(mobile ? row.locator(":scope > button") : row, mobile);
    }
  };
  await open();
  const dialog = page.getByRole("dialog");
  const query = dialog.getByPlaceholder('project = PROJ AND status = "Open"', { exact: true });
  await query.fill(rawOld);
  const method = create ? "POST" : "PATCH";
  const savePath = `${endpoint}${create ? "" : `/${original!.id}`}`;
  const heldWrite = await holdWrite(page, method, savePath, fixture.workspaceId);
  const { writes } = heldWrite;
  const errors: unknown[] = [];
  try {
    await press(
      dialog.getByRole("button", { name: create ? "Create" : "Update", exact: true }),
      mobile,
    );
    const saved = await heldWrite.admitted;
    fixture.checkpoint("held-accepted-write");
    expect(writes).toHaveLength(1);
    expect(writes[0]).toMatchObject({
      workflowId: fixture.workflowId,
      workflowStepId: fixture.stepId,
      executorProfileId: fixture.profileId,
      agentProfileId: seed.agentProfileId,
      enabled: false,
      jql: rawOld,
    });
    if (create)
      expect(writes[0]).toMatchObject({
        workspaceId: fixture.workspaceId,
      });
    expect((await readWatches(api, fixture.workspaceId)).find((w) => w.id === saved.id)?.jql).toBe(
      rawOld.trim(),
    );
    await press(dialog.getByRole("button", { name: "Cancel", exact: true }), mobile);
    await expect(dialog).toHaveCount(0);
    await open();
    await query.fill(rawNew);
    await press(query, mobile);
    await query.press("Home");
    await query.press("ArrowRight");
    const caret = await query.evaluate((el: HTMLInputElement) => [
      el.selectionStart,
      el.selectionEnd,
    ]);
    const ack = waitForHttp(page, method, new RegExp(`^${savePath}$`));
    await heldWrite.release();
    await ack;
    await expect(
      page.getByText(create ? "Watcher created" : "Watcher updated", { exact: true }),
    ).toBeVisible();
    await expect(watchRow(page, saved.id, mobile)).toContainText(rawOld.trim());
    await expect(query).toHaveValue(rawNew);
    await expect(query).toBeFocused();
    expect(
      await query.evaluate((el: HTMLInputElement) => [el.selectionStart, el.selectionEnd]),
    ).toEqual(caret);
    const nextSave = dialog.getByRole("button", {
      name: create ? "Create" : "Update",
      exact: true,
    });
    await expect(nextSave).toBeEnabled();
    expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(
      false,
    );
    await captureDraft(page, mobile, create);
    const nextAck = waitForHttp(page, method, new RegExp(`^${savePath}$`));
    await press(nextSave, mobile);
    await nextAck;
    await expect(dialog).toHaveCount(0);
    expect(writes).toHaveLength(2);
    expect(writes[1]).toMatchObject({ jql: rawNew });
    await page.reload();
    await expect(watchRow(page, saved.id, mobile)).toContainText(
      create ? rawOld.trim() : rawNew.trim(),
    );
  } catch (error) {
    errors.push(error);
  } finally {
    try {
      await heldWrite.cleanup();
    } catch (error) {
      if (!errors.includes(error)) errors.push(error);
    }
  }
  if (errors.length) throw new AggregateError(errors, "Jira watch flow and route cleanup failures");
}
