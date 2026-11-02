import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { MCPProfileOwnerPage } from "../../lib/types/http-mcp";
import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-002.1
export async function checkWorkspaceProfileDestination(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const response = await api.rawRequest(
    "POST",
    `/api/v1/office/workspaces/${seed.workspaceId}/agents`,
    {
      name: "Workspace documentation owner",
      role: "worker",
      agent_profile_id: seed.agentProfileId,
    },
  );
  const created = await response.json();
  expect(response.status, JSON.stringify(created)).toBe(201);
  const profileId = created.agent.id as string;
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "workspace-owner-tools",
    display_name: "Workspace documentation",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/owner" },
  });
  const capture = new PrAssetCapture(page, "mcp-workspace-profile.spec.ts", {
    captureKey: mobile ? "phone" : "desktop",
  });
  try {
    const inventoryResponse = await api.rawRequest(
      "GET",
      `/api/v1/workspaces/${seed.workspaceId}/mcp-profile-owners`,
    );
    expect(inventoryResponse.ok).toBe(true);
    const inventory = (await inventoryResponse.json()) as MCPProfileOwnerPage;
    const owner = inventory.items.find((item) => item.id === profileId)!;
    expect(owner.workspace_id).toBe(seed.workspaceId);
    expect(owner.agent_id).toBeTruthy();
    expect(
      inventory.items.some((item) => item.id === seed.agentProfileId && !item.workspace_id),
    ).toBe(true);
    expect(
      (await api.listAgents()).agents
        .flatMap((agent) => agent.profiles)
        .some((profile) => profile.id === profileId),
    ).toBe(false);
    await api.replaceMCPSelections("profile", profileId, seed.workspaceId, [server.id]);
    await page.goto(
      `/settings/workspaces/${seed.workspaceId}/mcp-servers?serverId=${server.id}&view=usage`,
    );
    await page
      .getByTestId("mcp-usage")
      .getByRole("link", { name: owner.name, exact: true })
      .click();
    const route = `/settings/agents/${owner.agent_id}/profiles/${profileId}?workspaceId=${seed.workspaceId}#mcp`;
    await expect(page).toHaveURL(
      new RegExp(`profiles/${profileId}\\?workspaceId=${seed.workspaceId}#mcp$`),
    );
    const destination = page.getByTestId("workspace-profile-mcp-destination");
    await expect(destination.getByRole("heading", { name: owner.name })).toBeVisible();
    const picker = page.getByTestId(`profile-mcp-selection-${profileId}`);
    const choice = picker.getByTestId(`mcp-choice-${server.id}`).getByRole("checkbox");
    await expect(choice).toBeChecked();
    await choice.click();
    expect(
      (await api.getMCPSelections("profile", profileId, seed.workspaceId)).definition_ids,
    ).toEqual([server.id]);
    await page.keyboard.press("Escape");
    await page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("profile", profileId, seed.workspaceId)).definition_ids,
      )
      .toEqual([]);
    await page.reload();
    await expect(destination).toBeVisible();
    await expect(choice).not.toBeChecked();
    if (capture.capturing) {
      await waitForFiniteAnimations(destination);
      await capture.screenshot("workspace-profile", {
        caption: `${mobile ? "Phone" : "Desktop"} authorized workspace-profile MCP editor with shared Save changes`,
      });
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
    const other = await api.createWorkspace("Unrelated profile context");
    try {
      await page.goto(route.replace(seed.workspaceId, other.id));
      await expect(page.getByRole("alert")).toContainText("destination is unavailable");
      await expect(destination).toHaveCount(0);
      const denied = await api.rawRequest(
        "GET",
        `/api/v1/workspaces/${other.id}/mcp-profile-owners/${profileId}`,
      );
      expect(denied.status).toBe(404);
    } finally {
      await api.deleteWorkspace(other.id, other.name);
    }
  } finally {
    await api.replaceMCPSelections("profile", profileId, seed.workspaceId, []);
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
    await api.rawRequest("DELETE", `/api/v1/office/agents/${profileId}`);
    capture.flush();
  }
}
