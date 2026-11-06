import { watchWs, type WsWatcher } from "./causal-waits";
import { waitForFiniteAnimations } from "./pr-capture";
import type { PrAssetCapture } from "./pr-asset-capture";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { Automation } from "@/lib/types/automation";
import type { BuiltinMCPManagementView } from "@/lib/types/builtin-mcp";
import { expect, type Page } from "@playwright/test";
export const OPTIONAL_AUTOMATION_PACKAGES = [
  "configuration",
  "rich-output",
  "walkthroughs",
  "diagnostics",
] as const;
export async function saveAutomationMCP(page: Page, watcher: WsWatcher) {
  const save = page.getByTestId("settings-floating-save");
  const updated = watcher.waitForResponse("automation.update");
  await Promise.all([
    updated,
    save.getByRole("button", { name: /^(Save changes|Retry save)$/ }).click(),
  ]);
  await expect(save).toHaveAttribute("data-status", "saved");
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
  const configuration = section
    .getByTestId("automation-mcp-package-configuration")
    .getByRole("checkbox");
  await expect(configuration).toBeChecked();
  await expect(configuration).toBeEnabled();
  await configuration.click();
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
