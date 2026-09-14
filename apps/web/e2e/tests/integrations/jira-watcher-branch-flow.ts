import { type Locator, type Page, expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";
import type { PrAssetCapture } from "../../helpers/pr-asset-capture";
import type { SeedData } from "../../fixtures/test-base";
import type { JiraIssueWatch } from "@/lib/types/jira";
import { JiraSettingsPage } from "../../pages/jira-settings-page";

const QUALIFIED_BRANCH = "origin/main";
const DEFAULT_JQL = 'project = PROJ AND status = "Open" ORDER BY created DESC';

type InputMode = "mouse" | "touch";

function comboboxByLabel(root: Locator, label: string): Locator {
  return root.getByText(label, { exact: true }).locator("xpath=..").getByRole("combobox");
}

async function activate(locator: Locator, inputMode: InputMode): Promise<void> {
  if (inputMode === "touch") {
    await locator.tap();
    return;
  }
  await locator.click();
}

async function listJiraWatchers(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<JiraIssueWatch[]> {
  const response = await apiClient.rawRequest(
    "GET",
    `/api/v1/jira/watches/issue?workspace_id=${encodeURIComponent(workspaceId)}`,
  );
  expect(response.ok).toBe(true);
  const body = (await response.json()) as { watches?: JiraIssueWatch[] };
  return body.watches ?? [];
}

async function deleteJiraWatchers(
  apiClient: ApiClient,
  workspaceId: string,
  watchers: JiraIssueWatch[],
): Promise<void> {
  for (const watcher of watchers) {
    const response = await apiClient.rawRequest(
      "DELETE",
      `/api/v1/jira/watches/issue/${encodeURIComponent(watcher.id)}?workspace_id=${encodeURIComponent(workspaceId)}`,
    );
    expect(response.ok).toBe(true);
  }
}

async function configureJira(apiClient: ApiClient, workspaceId: string): Promise<void> {
  await apiClient.mockJiraReset();
  await apiClient.mockJiraSetAuthResult({
    ok: true,
    displayName: "Jira E2E User",
    email: "e2e@example.com",
  });
  await apiClient.setJiraConfig({
    siteUrl: "https://acme.atlassian.net",
    email: "e2e@example.com",
    secret: "api-token-value",
    defaultProjectKey: "PROJ",
    workspaceId,
  });
  await apiClient.waitForIntegrationAuthHealthy("jira", { workspaceId });
}

async function choose(
  page: Page,
  dialog: Locator,
  label: string,
  option: string | RegExp,
  inputMode: InputMode,
): Promise<void> {
  await activate(comboboxByLabel(dialog, label), inputMode);
  const listbox = page.getByRole("listbox");
  await expect(listbox).toBeVisible();
  await activate(listbox.getByRole("option", { name: option }), inputMode);
}

async function assertWithinViewport(locator: Locator, page: Page): Promise<void> {
  const box = await locator.boundingBox();
  const viewport = page.viewportSize();
  expect(box).not.toBeNull();
  expect(viewport).not.toBeNull();
  if (!box || !viewport) return;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.y).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height);
}

export async function assertJiraWatcherQualifiedBranchPersists(args: {
  page: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  inputMode: InputMode;
  branchNames?: string[];
  prCapture?: PrAssetCapture;
}): Promise<void> {
  const { page, apiClient, seedData, inputMode, branchNames = [], prCapture } = args;
  const existingWatchers = await listJiraWatchers(apiClient, seedData.workspaceId);
  await deleteJiraWatchers(apiClient, seedData.workspaceId, existingWatchers);
  await configureJira(apiClient, seedData.workspaceId);

  try {
    const settings = new JiraSettingsPage(page);
    await settings.gotoWorkspace(seedData.workspaceId);

    await activate(page.getByRole("button", { name: "New watcher", exact: true }), inputMode);
    const dialog = page.getByRole("dialog");
    await expect(dialog).toBeVisible();

    await choose(page, dialog, "Workspace", "E2E Workspace", inputMode);
    await choose(page, dialog, "Workflow", "E2E Workflow", inputMode);
    const workflowStepTrigger = comboboxByLabel(dialog, "Workflow Step");
    await expect(workflowStepTrigger).toBeEnabled();
    await activate(workflowStepTrigger, inputMode);
    await expect
      .poll(async () => page.getByRole("listbox").getByRole("option").count())
      .toBeGreaterThan(0);
    const workflowStepOption = page
      .getByRole("listbox")
      .getByRole("option", { name: "Backlog", exact: true });
    await expect(workflowStepOption).toHaveCount(1);
    await activate(workflowStepOption, inputMode);

    await choose(page, dialog, "Repository", "E2E Repo", inputMode);
    const branchTrigger = page.getByTestId("watcher-base-branch-selector");
    await expect(branchTrigger).toBeEnabled();
    await activate(branchTrigger, inputMode);
    const branchDropdown = page.getByTestId("watcher-base-branch-dropdown");
    await expect(branchDropdown).toBeVisible();
    await expect(branchDropdown.getByRole("option", { name: /^main local/ })).toBeVisible();
    await expect(
      branchDropdown.getByRole("option", { name: /^origin\/main origin/ }),
    ).toBeVisible();

    const branchSearch = branchDropdown.getByPlaceholder("Search branches...");
    await branchSearch.fill(QUALIFIED_BRANCH);
    await expect(
      branchDropdown.getByRole("option", { name: /^origin\/main origin/ }),
    ).toBeVisible();
    await expect(branchDropdown.getByRole("option", { name: /^main local/ })).toHaveCount(0);
    await branchSearch.fill("");
    await prCapture?.screenshot(`jira-watcher-qualified-branch-${inputMode}`, {
      caption: `Jira watcher branch picker with local and ${QUALIFIED_BRANCH} choices`,
    });
    await activate(branchDropdown.getByRole("option", { name: /^origin\/main origin/ }), inputMode);
    await expect(branchDropdown).toBeHidden();

    const createButton = dialog.getByRole("button", { name: "Create", exact: true });
    await expect(createButton).toBeEnabled();
    await activate(createButton, inputMode);
    await expect(dialog).toBeHidden();

    let watcherId = "";
    await expect
      .poll(async () => {
        const watchers = await listJiraWatchers(apiClient, seedData.workspaceId);
        const watcher = watchers.find((candidate) => candidate.jql === DEFAULT_JQL);
        watcherId = watcher?.id ?? "";
        return watcher?.baseBranch ?? "";
      })
      .toBe(QUALIFIED_BRANCH);
    expect(watcherId).not.toBe("");

    await page.reload();
    await settings.siteInput.waitFor();
    const row = page.getByTestId(
      inputMode === "touch" ? `jira-watch-mobile-row-${watcherId}` : `jira-watch-row-${watcherId}`,
    );
    await expect(row).toBeVisible();
    if (inputMode === "touch") {
      const editButton = row.getByRole("button").filter({ hasText: DEFAULT_JQL });
      await expect(editButton).toHaveCount(1);
      await activate(editButton, inputMode);
    } else {
      await activate(row, inputMode);
    }

    const editDialog = page.getByRole("dialog", { name: /Edit Jira Watcher/i });
    await expect(editDialog).toBeVisible();
    const editBranchTrigger = page.getByTestId("watcher-base-branch-selector");
    await expect(editBranchTrigger).toBeEnabled();
    await expect(editBranchTrigger).toContainText(QUALIFIED_BRANCH);
    await activate(editBranchTrigger, inputMode);
    const editDropdown = page.getByTestId("watcher-base-branch-dropdown");
    await expect(editDropdown.getByRole("option", { name: /^origin\/main origin/ })).toBeVisible();

    if (inputMode === "touch") {
      await assertWithinViewport(editDialog, page);
      await assertWithinViewport(editDropdown, page);
      const remoteOption = editDropdown.getByRole("option", { name: /^origin\/main origin/ });
      const remoteBox = await remoteOption.boundingBox();
      expect(remoteBox).not.toBeNull();
      if (remoteBox) expect(remoteBox.height).toBeGreaterThanOrEqual(44);
      const branchList = editDropdown.locator('[data-slot="command-list"]');
      await expect
        .poll(async () =>
          branchList.evaluate((element) => element.scrollHeight > element.clientHeight),
        )
        .toBe(branchNames.length === 0 ? false : true);
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
        page.viewportSize()?.width ?? 390,
      );
    }

    await prCapture?.screenshot(`jira-watcher-qualified-branch-reloaded-${inputMode}`, {
      caption: `Saved Jira watcher reopened with ${QUALIFIED_BRANCH} selected`,
    });
    await page.keyboard.press("Escape");
  } finally {
    const remainingWatchers = await listJiraWatchers(apiClient, seedData.workspaceId);
    await deleteJiraWatchers(
      apiClient,
      seedData.workspaceId,
      remainingWatchers.filter((watcher) => watcher.jql === DEFAULT_JQL),
    );
  }
}
