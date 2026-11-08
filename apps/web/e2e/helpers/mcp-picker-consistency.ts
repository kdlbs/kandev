import { expect, type Locator, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export async function expectMCPChoiceDetails(
  row: Locator,
  description: string,
  transport = "Server-sent events",
) {
  await expect(row).toHaveCount(1);
  await expect(row.getByTestId("mcp-definition-description")).toHaveText(description);
  const badges = row.getByTestId("mcp-definition-badges");
  await expect(badges).toContainText("Remote endpoint");
  await expect(badges).toContainText(transport);
  await expect(badges).toContainText("Custom");
  const checkbox = row.getByRole("checkbox");
  const describedBy = await checkbox.getAttribute("aria-describedby");
  expect(describedBy).toBeTruthy();
  for (const badge of await badges.locator('[data-slot="badge"]').all()) {
    await expect(badge).not.toHaveAttribute("role", "button");
    const box = (await badge.boundingBox())!;
    const parent = (await row.boundingBox())!;
    expect(box.x).toBeGreaterThanOrEqual(parent.x - 1);
    expect(box.x + box.width).toBeLessThanOrEqual(parent.x + parent.width + 1);
  }
}

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.7, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.21
export async function checkRepositoryMCPPicker(
  page: Page,
  apiClient: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const previous = await apiClient.getMCPSelections(
    "repository",
    seed.repositoryId,
    seed.workspaceId,
  );
  const definitions = [];
  try {
    definitions.push(
      await apiClient.createMCPServer(seed.workspaceId, {
        runtime_name: "e2e-mcp-picker-rich",
        display_name: "Atlas documentation",
        description: "Shared project guides for the Atlas infrastructure team.",
        source: "custom",
        execution_mode: "remote",
        transport: "sse",
        configuration: { url: "https://mcp.example.test/picker" },
      }),
    );
    definitions.push(
      await apiClient.createMCPServer(seed.workspaceId, {
        runtime_name: "e2e-mcp-picker-disabled",
        display_name: "Archived tools",
        description: "   ",
        source: "custom",
        execution_mode: "existing_executable",
        transport: "stdio",
        configuration: { command: "archived-tool" },
        enabled: false,
      }),
    );
    await page.goto(`/settings/workspaces/${seed.workspaceId}/repositories`);
    await page.getByRole("heading", { name: "E2E Repo", exact: true }).click();
    const trigger = page.getByTestId("repository-mcp-selection-trigger");
    await expect(trigger).toHaveAttribute("aria-expanded", "false");
    await trigger.click();
    const content = page.getByTestId("repository-mcp-selection-content");
    const search = content.getByRole("textbox", { name: "Search MCP servers" });
    await expect(search).toBeVisible();
    await expect(content.getByRole("button", { name: /^MCP servers/ })).toHaveCount(0);
    const row = content.getByTestId(`mcp-choice-${definitions[0].id}`);
    await row.scrollIntoViewIfNeeded();
    await expectMCPChoiceDetails(row, definitions[0].description!);
    const disabled = content.getByTestId(`mcp-choice-${definitions[1].id}`);
    await expect(disabled.getByRole("checkbox")).toBeDisabled();
    await expect(disabled.getByTestId("mcp-definition-description")).toHaveCount(0);
    await expect(disabled.getByTestId("mcp-definition-badges")).toContainText(
      "Existing executable",
    );
    if (mobile) expect((await row.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await search.fill("e2e-mcp-picker-rich");
    await row.getByRole("checkbox").focus();
    await page.keyboard.press("Space");
    const floating = page.getByTestId("settings-floating-save");
    await floating.getByRole("button", { name: "Save changes", exact: true }).click();
    await expect
      .poll(
        async () =>
          (await apiClient.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
            .definition_ids,
      )
      .toContain(definitions[0].id);
    await page.reload();
    await page.getByRole("heading", { name: "E2E Repo", exact: true }).click();
    await trigger.click();
    await expect(row.getByRole("checkbox")).toBeChecked();
    await row.getByRole("checkbox").click();
    await floating.getByRole("button", { name: "Reset", exact: true }).click();
    await expect(row.getByRole("checkbox")).toBeChecked();
    await search.fill("no matching server");
    await expect(content.getByRole("checkbox")).toHaveCount(0);
    await search.fill("");
    await content.getByRole("checkbox").last().scrollIntoViewIfNeeded();
    await assertNoDocumentHorizontalOverflow(page, "repository MCP picker");
  } finally {
    await apiClient.replaceMCPSelections(
      "repository",
      seed.repositoryId,
      seed.workspaceId,
      previous.definition_ids,
    );
    for (const definition of definitions.reverse())
      await apiClient.deleteMCPServer(seed.workspaceId, definition.id, definition.revision);
  }
}
