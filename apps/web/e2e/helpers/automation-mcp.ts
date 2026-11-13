import { watchWs, type WsWatcher } from "./causal-waits";
import { waitForFiniteAnimations } from "./pr-capture";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Automation } from "@/lib/types/automation";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";
import { expect, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { mkdtempSync } from "node:fs";
import { join } from "node:path";
import type { BackendContext } from "../fixtures/backend";
export const OPTIONAL_AUTOMATION_PACKAGES = [
  "configuration",
  "rich-output",
  "walkthroughs",
  "diagnostics",
] as const;
/** Choose a package through the same list/detail interaction used by task settings. */
export async function chooseAutomationPackage(
  page: Page,
  id: string,
  enabled: boolean,
  touch = false,
) {
  const row = page.getByTestId(`automation-mcp-package-${id}`);
  if (touch) await row.tap();
  else await row.click();
  const detail = page.getByTestId("automation-mcp-package-detail");
  const choice = detail.getByRole("combobox");
  if (touch) await choice.tap();
  else await choice.click();
  const option = page.getByRole("option", { name: enabled ? "On" : "Off", exact: true });
  if (touch) await option.tap();
  else await option.click();
  const back = detail.getByRole("button", { name: "Back to packages", exact: true });
  if (touch) await back.tap();
  else await back.click();
  await expect(row).toBeFocused();
  await expect(row.locator('[data-slot="badge"]')).toHaveText(enabled ? "On" : "Off");
}

export async function saveAutomationMCP(page: Page, watcher: WsWatcher) {
  const save = page.getByTestId("settings-floating-save");
  const updated = watcher.waitForResponse("automation.update");
  await Promise.all([
    updated,
    save.getByRole("button", { name: /^(Save changes|Retry save)$/ }).click(),
  ]);
  await expect(save).toHaveAttribute("data-status", "saved");
}

/** Keep inheritance aligned with the single repository retained when a loaded automation is saved. */
export async function verifyAutomationMCPRepositoryInheritance({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
  touch = false,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  backend: BackendContext;
  prCapture: PrAssetCapture;
  touch?: boolean;
}) {
  const workspace = await apiClient.createWorkspace("Loaded MCP inheritance workspace");
  const primary = await apiClient.createRepository(workspace.id, seedData.repositoryPath);
  const path = mkdtempSync(join(backend.tmpDir, "mcp-inheritance-"));
  execFileSync("git", ["clone", "--local", seedData.repositoryPath, path]);
  const omitted = await apiClient.createRepository(workspace.id, path, "main", {
    name: "Omitted repository",
  });
  const createServer = (role: string) =>
    apiClient.createMCPServer(workspace.id, {
      runtime_name: `${role.toLowerCase()}-repository-reports`,
      display_name: `${role} repository reports`,
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://reports.example.test/mcp" },
      source: "custom",
    });
  const server = await createServer("Omitted");
  const retained = await createServer("Retained");
  await apiClient.replaceMCPSelections("repository", omitted.id, workspace.id, [server.id]);
  await apiClient.replaceMCPSelections("repository", primary.id, workspace.id, [retained.id]);
  const automation = await apiClient.seedAutomation({
    workspaceId: workspace.id,
    name: "Loaded repository inheritance",
    repositoryMode: "selected",
    repositoryIds: [primary.id, omitted.id],
    agentProfileId: seedData.agentProfileId,
  });
  const loaded = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  expect(loaded.repository_ids).toEqual([primary.id, omitted.id]);
  const watcher = watchWs(testPage);
  await testPage.goto(`/settings/workspaces/${workspace.id}/automations/${automation.id}`);
  const section = testPage.getByTestId("automation-mcp-access");
  const direct = section.getByTestId(`automation-mcp-server-${server.id}`).getByRole("checkbox");
  await expect(section.getByText("Retained repository reports (inherited)")).toBeVisible();
  await expect(direct).toBeVisible();
  await expect(direct).toBeEnabled();
  await expect(section.getByText("Omitted repository reports (inherited)")).toHaveCount(0);
  if (touch) await direct.tap();
  else await direct.click();
  await expect(direct).toBeChecked();
  await section.evaluate((element) => element.scrollIntoView({ block: "start" }));
  await prCapture.screenshot("automation-mcp-repository-inheritance", {
    caption: "MCP inheritance follows the repositories that the automation will save",
  });
  await saveAutomationMCP(testPage, watcher);
  const saved = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  expect(saved.repository_ids).toEqual([primary.id]);
  expect(saved.mcp_selection?.definition_ids).toEqual([server.id]);
  await testPage.reload();
  await expect(direct).toBeChecked();
  expect(
    (await apiClient.getMCPSelections("repository", omitted.id, workspace.id)).definition_ids,
  ).toEqual([server.id]);
}

export async function verifyAutomationMCPRecovery({
  testPage,
  apiClient,
  seedData,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
}) {
  const watcher = watchWs(testPage);
  const workspace = await apiClient.createWorkspace("MCP recovery workspace");
  const workflow = await apiClient.createWorkflow(workspace.id, "MCP recovery workflow", "simple");
  const recovery = { ...seedData, workspaceId: workspace.id, workflowId: workflow.id };
  const server = await apiClient.createMCPServer(recovery.workspaceId, {
    runtime_name: "recovery-reports",
    display_name: "Recovery reports",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://reports.example.test/mcp" },
    source: "custom",
  });
  await apiClient.replaceMCPSelections("profile", recovery.agentProfileId, recovery.workspaceId, [
    server.id,
  ]);
  const automation = await apiClient.seedAutomation({
    workspaceId: recovery.workspaceId,
    name: "Recover MCP choices",
    workflowId: recovery.workflowId,
    taskMode: "normal_task",
    repositoryMode: "none",
    agentProfileId: recovery.agentProfileId,
  });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    mcp_selection: { builtin_packages: ["core", "configuration"], definition_ids: [server.id] },
    expected_mcp_selection_revision: 0,
  });
  const policyResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${recovery.workspaceId}/mcp-built-in`,
  );
  const view = (await policyResponse.json()) as BuiltinMCPManagementView;
  const blocked = await apiClient.rawRequest(
    "PUT",
    `/api/v1/workspaces/${recovery.workspaceId}/mcp-built-in/policy`,
    {
      version: 1,
      expected_revision: view.policy.revision,
      packages: { ...view.policy.packages, configuration: "blocked" },
    },
  );
  expect(blocked.ok).toBe(true);
  await testPage.goto(`/settings/workspaces/${recovery.workspaceId}/automations/${automation.id}`);
  const section = testPage.getByTestId("automation-mcp-access");
  const configuration = section.getByTestId("automation-mcp-package-configuration");
  await expect(configuration.locator('[data-slot="badge"]')).toHaveText("On");
  await chooseAutomationPackage(testPage, "configuration", false);
  await configuration.click();
  await section.getByRole("combobox").click();
  await expect(testPage.getByRole("option", { name: "On", exact: true })).toBeDisabled();
  await testPage.keyboard.press("Escape");
  await section.getByRole("button", { name: "Back to packages", exact: true }).click();
  const direct = section.getByTestId(`automation-mcp-server-${server.id}`).getByRole("checkbox");
  await expect(direct).toBeChecked();
  await direct.click();
  await saveAutomationMCP(testPage, watcher);
  const saved = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  expect(saved.mcp_selection).toEqual({ builtin_packages: ["core"], definition_ids: [] });
  expect(
    (await apiClient.getMCPSelections("profile", recovery.agentProfileId, recovery.workspaceId))
      .definition_ids,
  ).toContain(server.id);
  await expect(section.getByText("Recovery reports (inherited)")).toBeVisible();
}

export async function verifyAutomationMCPDeleteWarning({
  testPage,
  apiClient,
  seedData,
  prCapture,
}: {
  testPage: Page;
  apiClient: ApiClient;
  seedData: SeedData;
  prCapture: PrAssetCapture;
}) {
  const server = await apiClient.createMCPServer(seedData.workspaceId, {
    runtime_name: "automation-delete-report",
    display_name: "Automation report server",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://reports.example.test/mcp" },
    source: "custom",
  });
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Retained selection",
  });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    mcp_selection: { builtin_packages: ["core"], definition_ids: [server.id] },
    expected_mcp_selection_revision: 0,
  });
  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
  await testPage
    .getByTestId(`mcp-server-card-${server.id}`)
    .getByRole("button", { name: "Delete MCP server" })
    .click();
  const dialog = testPage.getByRole("dialog");
  const warning = dialog.getByTestId("mcp-automation-delete-impact");
  await expect(dialog.getByTestId("mcp-selection-impact")).toContainText("1 automation");
  await expect(warning).toContainText("Automations keep their saved selection after deletion.");
  await expect(warning).toContainText("New agent processes cannot attach this server.");
  const confirm = dialog.getByRole("button", { name: "Delete", exact: true });
  await expect(confirm).toBeInViewport();
  await waitForFiniteAnimations(dialog);
  await prCapture.screenshot("automation-mcp-delete-warning", {
    caption: "Deletion retains automation selections and prevents new server attachments",
  });
  await confirm.click();
  await expect(testPage.getByTestId(`mcp-server-card-${server.id}`)).toHaveCount(0);
  const saved = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  expect(saved.mcp_selection?.definition_ids).toEqual([server.id]);
}
