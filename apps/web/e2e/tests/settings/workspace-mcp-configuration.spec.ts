import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";

const TEST_RUNTIME_PREFIX = "e2e-mcp-";

async function cleanupMCPServers(apiClient: ApiClient, workspaceId: string) {
  const servers = await apiClient.listMCPServers(workspaceId);
  for (const server of servers) {
    if (server.runtime_name.startsWith(TEST_RUNTIME_PREFIX)) {
      await apiClient.deleteMCPServer(workspaceId, server.id, server.revision);
    }
  }
}

test.describe("Workspace MCP configuration", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupMCPServers(apiClient, seedData.workspaceId);
  });

  test("shows a workspace-owned MCP definition in the configured view", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await apiClient.createMCPServer(seedData.workspaceId, {
      runtime_name: `${TEST_RUNTIME_PREFIX}docs-remote`,
      display_name: "Docs remote MCP",
      description: "A remote MCP definition for documentation tasks.",
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://mcp.example.test/docs" },
      source: "custom",
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);

    const settings = testPage.getByTestId("workspace-mcp-settings");
    await expect(settings).toBeVisible();
    await expect(testPage.getByTestId(`mcp-server-card-${server.id}`)).toContainText(
      "Docs remote MCP",
    );
    await expect(settings).toContainText("Remote endpoint");
    await expect(settings).toContainText("Streamable HTTP");
    await expect(testPage.getByTestId("mcp-selection-impact")).toContainText(
      "Not selected in any profile",
    );
  });

  test("creates a remote definition through the shared settings save control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
    await testPage.getByRole("button", { name: "Add MCP server" }).first().click();

    const form = testPage.getByTestId("mcp-definition-form");
    await expect(form).toBeVisible();
    await form.getByLabel("Runtime name").fill(`${TEST_RUNTIME_PREFIX}ui-remote`);
    await form.getByLabel("Display name").fill("UI remote MCP");
    await form.getByLabel("Description").fill("Saved from workspace settings.");
    await form.getByLabel("Setup mode").click();
    await testPage.getByRole("option", { name: "Remote endpoint" }).click();
    await form.getByLabel("Remote URL").fill("https://mcp.example.test/ui");

    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    await save.click();
    await expect(form).toHaveCount(0);

    const servers = await apiClient.listMCPServers(seedData.workspaceId);
    const created = servers.find(
      (server) => server.runtime_name === `${TEST_RUNTIME_PREFIX}ui-remote`,
    );
    expect(created).toMatchObject({
      display_name: "UI remote MCP",
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://mcp.example.test/ui" },
    });
  });

  test("reviews and saves an exact-version curated npm setup without materializing it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marketplace = await apiClient.searchMCPMarketplace("example");
    const entry = marketplace.entries.find((item) => item.name === "com.kandev/example-tools");
    expect(entry).toBeDefined();
    const choice = entry?.choices.find((item) => item.kind === "package" && item.selectable);
    expect(choice).toMatchObject({
      identifier: "@kandev/example-tools",
      version: "1.0.0",
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
    await testPage.getByRole("tab", { name: "Marketplace" }).click();
    const marketplaceView = testPage.getByTestId("mcp-marketplace");
    await marketplaceView.getByRole("textbox", { name: "Search MCP marketplace" }).fill("example");
    await marketplaceView.getByRole("textbox", { name: "Search MCP marketplace" }).press("Enter");
    const card = marketplaceView.locator('[data-slot="card"]').filter({ hasText: "Example tools" });
    await expect(card).toBeVisible();
    await card.getByRole("button", { name: "Review" }).click();

    const review = testPage.getByRole("dialog");
    await expect(review).toContainText("@kandev/example-tools@1.0.0");
    await expect(review).toContainText("Kandev-curated template");
    await review
      .getByRole("textbox", { name: "Runtime name" })
      .fill(`${TEST_RUNTIME_PREFIX}example-tools`);
    await review.getByRole("button", { name: "Save setup" }).click();
    await expect(review).toBeHidden();

    await expect
      .poll(async () => {
        const servers = await apiClient.listMCPServers(seedData.workspaceId);
        return servers.find(
          (server) => server.runtime_name === `${TEST_RUNTIME_PREFIX}example-tools`,
        );
      })
      .toMatchObject({
        source: "registry",
        source_identity: "com.kandev/example-tools@1.0.0",
        execution_mode: "managed_package",
        configuration: {
          package_type: "npm",
          package_name: "@kandev/example-tools",
          package_version: "1.0.0",
        },
      });
  });

  test("shows unsupported marketplace choices and selection impact before delete", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const server = await apiClient.createMCPServer(seedData.workspaceId, {
      runtime_name: `${TEST_RUNTIME_PREFIX}selected`,
      display_name: "Selected MCP",
      description: "A selected server for the delete guard.",
      execution_mode: "remote",
      transport: "streamable_http",
      configuration: { url: "https://mcp.example.test/selected" },
      source: "custom",
    });
    const task = await apiClient.createTask(seedData.workspaceId, "MCP impact task", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    });
    await apiClient.replaceMCPSelections("profile", seedData.agentProfileId, seedData.workspaceId, [
      server.id,
    ]);
    await apiClient.replaceMCPSelections(
      "repository",
      seedData.repositoryId,
      seedData.workspaceId,
      [server.id],
    );
    await apiClient.replaceMCPSelections("task", task.id, seedData.workspaceId, [server.id]);

    await testPage.route("**/api/v1/mcp-marketplace*", async (route) => {
      await route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({
          entries: [
            {
              name: "com.example/unsupported",
              title: "Unsupported package",
              description: "This choice is intentionally unsupported.",
              version: "1.0.0",
              status: "active",
              revision: 1,
              source: "registry",
              publisher_supplied: true,
              trust_notice: "Publisher-supplied metadata. This is not a Kandev security review.",
              choices: [
                {
                  id: "package-0",
                  kind: "package",
                  registry_type: "pypi",
                  identifier: "unsupported-tools",
                  version: "1.0.0",
                  transport: "stdio",
                  selectable: false,
                  unsupported_reason: "No materializer is available for this package type",
                },
              ],
            },
          ],
          stale: true,
          degraded: true,
        }),
      });
    });

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
    const configured = testPage.getByTestId("workspace-mcp-settings");
    await expect(configured.getByTestId(`mcp-server-card-${server.id}`)).toContainText(
      "Selected in 1 profiles, 1 repositories, 1 tasks, and 0 sessions.",
    );
    await configured.getByRole("tab", { name: "Marketplace" }).click();
    const marketplaceView = testPage.getByTestId("mcp-marketplace");
    await expect(marketplaceView).toContainText("temporarily degraded");
    await expect(marketplaceView).toContainText("Showing last-good cached results");
    const unsupportedCard = marketplaceView
      .locator('[data-slot="card"]')
      .filter({ hasText: "Unsupported package" });
    await unsupportedCard.getByRole("button", { name: "Review" }).click();
    const review = testPage.getByRole("dialog");
    await expect(review).toContainText("No materializer is available");
    await expect(review.getByRole("button", { name: "Save setup" })).toBeDisabled();

    await testPage.keyboard.press("Escape");
    await expect(review).toBeHidden();
    await configured.getByRole("tab", { name: "Configured" }).click();
    await configured
      .getByTestId(`mcp-server-card-${server.id}`)
      .getByRole("button", {
        name: "Delete MCP server",
      })
      .click();
    const deleteDialog = testPage.getByRole("dialog");
    await expect(deleteDialog).toContainText("Selected in 1 profiles");
    await expect(deleteDialog).toContainText(
      "Any profile, repository, task, or session selections",
    );
    await deleteDialog.getByRole("button", { name: "Delete" }).click();
    await expect(testPage.getByTestId(`mcp-server-card-${server.id}`)).toHaveCount(0);
    await expect
      .poll(
        async () =>
          (await apiClient.getMCPSelections("task", task.id, seedData.workspaceId)).definition_ids,
      )
      .toEqual([]);
  });
});
