import { expect, type Locator, type Page, type Route } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { WatchFixture } from "./watch-dialog-ownership-fixture";
import { apiJson } from "./watch-dialog-ownership-fixture";
import { GitLabSettingsPage } from "../../pages/gitlab-settings-page";
import { waitForHttp } from "../../helpers/causal-waits";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

type Kind = "review" | "issue";
type Watch = {
  id: string;
  custom_query: string;
  workspace_id: string;
  executor_profile_id: string;
};
const rawOld = "  state=opened&search=submitted  ";
const rawNew = "  state=opened&search=still-unsent  ";
async function activate(locator: Locator, mobile: boolean) {
  await expect(locator).toHaveCount(1);
  if (mobile) await locator.tap();
  else await locator.click();
}
async function choose(page: Page, label: string, name: string, mobile: boolean) {
  await activate(page.getByRole("dialog").getByLabel(label, { exact: true }), mobile);
  await activate(page.getByRole("listbox").getByRole("option", { name, exact: true }), mobile);
}
async function openCreate(page: Page, kind: Kind, f: WatchFixture, mobile: boolean) {
  const settings = new GitLabSettingsPage(page);
  const section = kind === "review" ? settings.reviewSection() : settings.issueSection();
  await activate(section.getByRole("button", { name: "New watch", exact: true }), mobile);
  await choose(page, "Workflow", f.name, mobile);
  await choose(page, "Workflow step", `${f.name} step`, mobile);
  await choose(page, "Agent profile", f.agentLabel, mobile);
  await choose(page, "Executor profile", f.profileName, mobile);
}
async function readWatches(api: ApiClient, kind: Kind, workspaceId: string) {
  return (
    await apiJson<{ watches: Watch[] }>(
      api,
      "GET",
      `/api/v1/gitlab/watches/${kind}?workspace_id=${workspaceId}`,
    )
  ).watches;
}

export async function watchOpeningFlow(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  {
    fixture: f,
    mobile,
    kind,
    create,
  }: { fixture: WatchFixture; mobile: boolean; kind: Kind; create: boolean },
) {
  const endpoint = `/api/v1/gitlab/watches/${kind}`;
  let watch: Watch | undefined;
  if (!create) {
    watch = await apiJson<Watch>(api, "POST", `${endpoint}?workspace_id=${f.workspaceId}`, {
      workspace_id: f.workspaceId,
      workflow_id: f.workflowId,
      workflow_step_id: f.stepId,
      agent_profile_id: seed.agentProfileId,
      executor_profile_id: f.profileId,
      repository_id: f.repositoryId,
      projects: [{ path: "group/owned-watch" }],
      prompt: "Investigate {{issue.url}}",
      enabled: false,
      custom_query: "state=opened",
      poll_interval_seconds: 300,
      cleanup_policy: "auto",
      review_scope: "user",
      labels: [],
    });
  }
  const settings = new GitLabSettingsPage(page);
  await settings.goto(f.workspaceId);
  const open = async () => {
    if (create) return openCreate(page, kind, f, mobile);
    const card = kind === "review" ? settings.reviewWatches : settings.issueWatches;
    const surface = card.getByTestId(
      mobile ? "gitlab-watch-mobile-list" : "gitlab-watch-desktop-table",
    );
    await activate(surface.getByRole("button", { name: "Edit watch", exact: true }), mobile);
  };
  await open();
  const dialog = page.getByRole("dialog");
  const input = dialog.getByLabel("GitLab query parameters", { exact: true });
  await expect(dialog).toHaveCount(1);
  await input.fill(rawOld);
  const method = create ? "POST" : "PUT";
  const savePath = create ? endpoint : `${endpoint}/${watch!.id}`;
  const persisted = Promise.withResolvers<{
    payload: Record<string, unknown>;
    originalId: string;
  }>();
  const release = Promise.withResolvers<void>();
  const finished = Promise.withResolvers<void>();
  let held = false;
  let routeError: unknown;
  const handler = async (route: Route) => {
    if (route.request().method() !== method || held) return route.continue();
    held = true;
    try {
      const response = await route.fetch();
      expect(response.ok(), `Held response ${response.status()}: ${await response.text()}`).toBe(
        true,
      );
      expect(new URL(route.request().url()).searchParams.get("workspace_id")).toBe(f.workspaceId);
      const saved = (await response.json()) as Watch;
      persisted.resolve({ payload: route.request().postDataJSON(), originalId: saved.id });
      await release.promise;
      await route.fulfill({ response });
    } catch (error) {
      routeError = error;
      persisted.reject(error);
      await route.abort();
    } finally {
      finished.resolve();
    }
  };
  await page.route(`**${savePath}?*`, handler);
  try {
    const save = dialog.getByRole("button", {
      name: create ? "Create watch" : "Update watch",
      exact: true,
    });
    await activate(save, mobile);
    const original = await persisted.promise;
    expect(original.payload.custom_query).toBe(rawOld.trim());
    expect(original.payload.workflow_id).toBe(f.workflowId);
    expect(original.payload.workflow_step_id).toBe(f.stepId);
    expect(original.payload.executor_profile_id).toBe(f.profileId);
    expect(original.payload.workspace_id).toBe(create ? f.workspaceId : undefined);
    expect(
      (await readWatches(api, kind, f.workspaceId)).find((row) => row.id === original.originalId)
        ?.custom_query,
    ).toBe(rawOld.trim());
    await activate(dialog.getByRole("button", { name: "Cancel", exact: true }), mobile);
    await expect(dialog).toHaveCount(0);
    await open();
    await expect(dialog).toHaveCount(1);
    await input.fill(rawNew);
    await activate(input, mobile);
    await input.press("Home");
    await input.press("ArrowRight");
    const caret = await input.evaluate((element: HTMLInputElement) => [
      element.selectionStart,
      element.selectionEnd,
    ]);
    const ack = waitForHttp(page, method, new RegExp(`^${savePath}$`));
    release.resolve();
    await ack;
    await finished.promise;
    await expect(
      page.getByText(
        `${kind === "review" ? "Review" : "Issue"} watch ${create ? "created" : "updated"}`,
        { exact: true },
      ),
    ).toBeVisible();
    await expect(dialog).toHaveCount(1);
    await expect(input).toHaveValue(rawNew);
    await expect(input).toBeFocused();
    expect(
      await input.evaluate((element: HTMLInputElement) => [
        element.selectionStart,
        element.selectionEnd,
      ]),
    ).toEqual(caret);
    await assertNoDocumentHorizontalOverflow(page, "current GitLab watch draft");
    const next = waitForHttp(page, method, new RegExp(`^${savePath}$`));
    await activate(
      dialog.getByRole("button", { name: create ? "Create watch" : "Update watch", exact: true }),
      mobile,
    );
    await next;
    await expect(dialog).toHaveCount(0);
    const rows = await readWatches(api, kind, f.workspaceId);
    expect(
      rows.some(
        (row) => row.custom_query === rawNew.trim() && row.executor_profile_id === f.profileId,
      ),
    ).toBe(true);
    if (create)
      expect(rows.find((row) => row.id === original.originalId)?.custom_query).toBe(rawOld.trim());
    await page.reload();
    if (create) {
      expect(
        (await readWatches(api, kind, f.workspaceId)).map((row) => row.custom_query).sort(),
      ).toEqual([rawOld.trim(), rawNew.trim()].sort());
    } else {
      await open();
      await expect(input).toHaveValue(rawNew.trim());
      await activate(dialog.getByRole("button", { name: "Cancel", exact: true }), mobile);
    }
  } finally {
    release.resolve();
    if (held) await finished.promise;
    await page.unroute(`**${savePath}?*`, handler);
    await page.goto("about:blank");
    if (routeError) throw routeError;
  }
}
