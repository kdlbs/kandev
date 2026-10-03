import { expect, test } from "../../fixtures/test-base";
import type { ApiClient } from "../../helpers/api-client";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "../../helpers/layout-assertions";

const TEST_RUNTIME_PREFIX = "e2e-mcp-mobile-";

async function cleanupMCPServers(apiClient: ApiClient, workspaceId: string) {
  const servers = await apiClient.listMCPServers(workspaceId);
  for (const server of servers) {
    if (server.runtime_name.startsWith(TEST_RUNTIME_PREFIX)) {
      await apiClient.deleteMCPServer(workspaceId, server.id, server.revision);
    }
  }
}

test.describe("Workspace MCP configuration on mobile", () => {
  test.afterEach(async ({ apiClient, seedData }) => {
    await cleanupMCPServers(apiClient, seedData.workspaceId);
  });

  test("opens the full-height setup surface with labeled touch controls", async ({
    testPage,
    seedData,
  }) => {
    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
    const settings = testPage.getByTestId("workspace-mcp-settings");
    await expect(settings).toBeVisible();

    const add = settings.getByRole("button", { name: "Add MCP server" }).first();
    const addBox = await add.boundingBox();
    expect(addBox).not.toBeNull();
    expect(addBox!.height).toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(add, "mobile add MCP server");
    await add.tap();

    const form = testPage.getByTestId("mcp-definition-form");
    await expect(form).toBeVisible();
    await assertLocatorWithinViewportX(form, "mobile MCP definition form");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile MCP definition form");
    await expect(form.getByLabel("Runtime name")).toBeVisible();
    await expect(form.getByLabel("Display name")).toBeVisible();
    await expect(form.getByLabel("Setup mode")).toBeVisible();
    await form.getByLabel("Runtime name").fill(`${TEST_RUNTIME_PREFIX}draft`);
    await form.getByLabel("Display name").fill("Mobile draft MCP");

    const save = testPage
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    const saveBox = await save.boundingBox();
    expect(saveBox).not.toBeNull();
    expect(Math.round(saveBox!.height)).toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(save, "mobile MCP save control");
  });

  test("uses a marketplace review dialog that fits the phone viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const marketplace = await apiClient.searchMCPMarketplace("example");
    expect(marketplace.entries.some((entry) => entry.name === "com.kandev/example-tools")).toBe(
      true,
    );

    await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/mcp-servers`);
    const settings = testPage.getByTestId("workspace-mcp-settings");
    await settings.getByRole("tab", { name: "Marketplace" }).tap();
    const view = testPage.getByTestId("mcp-marketplace");
    const search = view.getByRole("textbox", { name: "Search MCP marketplace" });
    await search.fill("example");
    await search.press("Enter");

    const card = view.locator('[data-slot="card"]').filter({ hasText: "Example tools" });
    await expect(card).toBeVisible();
    const reviewButton = card.getByRole("button", { name: "Review" });
    const reviewButtonBox = await reviewButton.boundingBox();
    expect(reviewButtonBox).not.toBeNull();
    expect(reviewButtonBox!.height).toBeGreaterThanOrEqual(44);
    await reviewButton.tap();

    const dialog = testPage.getByRole("dialog");
    await expect(dialog).toBeVisible();
    const dialogBox = await dialog.boundingBox();
    const viewport = testPage.viewportSize();
    expect(dialogBox).not.toBeNull();
    expect(viewport).not.toBeNull();
    expect(dialogBox!.x).toBeGreaterThanOrEqual(-1);
    expect(dialogBox!.x + dialogBox!.width).toBeLessThanOrEqual(viewport!.width + 1);
    expect(dialogBox!.y).toBeGreaterThanOrEqual(-1);
    expect(dialogBox!.y + dialogBox!.height).toBeLessThanOrEqual(viewport!.height + 1);
    await expect(dialog.getByLabel("Runtime name")).toBeVisible();
    await expect(dialog).toContainText("@kandev/example-tools@1.0.0");

    const choiceButton = dialog.locator("button").filter({ hasText: "@kandev/example-tools" });
    const choiceBox = await choiceButton.boundingBox();
    expect(choiceBox).not.toBeNull();
    expect(choiceBox!.height).toBeGreaterThanOrEqual(44);
    const save = dialog.getByRole("button", { name: "Save setup" });
    await expect
      .poll(async () => Math.round((await save.boundingBox())?.height ?? 0))
      .toBeGreaterThanOrEqual(44);
    await assertLocatorWithinViewportX(dialog, "mobile MCP marketplace review");
    await assertNoDocumentHorizontalOverflow(testPage, "mobile MCP marketplace review");
  });
});
