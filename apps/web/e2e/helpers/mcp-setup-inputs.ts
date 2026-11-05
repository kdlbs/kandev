import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import {
  assertLocatorWithinViewportX,
  assertNoDocumentHorizontalOverflow,
} from "./layout-assertions";

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-001.5, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4
export async function checkMCPSetupInputs(
  page: Page,
  api: ApiClient,
  workspaceId: string,
  mobile: boolean,
) {
  const args = ["server.js", "Hello World", "", "  keep spaces  ", "C:\\Program Files\\MCP"];
  const secret = await api.createSecret("MCP setup credential", "synthetic-mcp-input-secret", {
    scope: "workspace",
    workspaceId,
  });
  const server = await api.createMCPServer(workspaceId, {
    runtime_name: "e2e-mcp-input-roundtrip",
    display_name: "MCP setup roundtrip",
    execution_mode: "existing_executable",
    transport: "stdio",
    configuration: { command: "node", args },
    source: "custom",
  });
  try {
    await page.goto(`/settings/workspaces/${workspaceId}/mcp-servers`);
    const edit = page
      .getByTestId(`mcp-server-card-${server.id}`)
      .getByRole("button", { name: "Edit", exact: true });
    if (mobile) await edit.tap();
    else await edit.click();
    const form = page.getByTestId("mcp-definition-form");
    await expect(form.getByLabel("Argument 3", { exact: true })).toHaveValue("");
    await form.getByLabel("Description").fill("Edited without changing the argument boundaries.");
    const environment = form
      .locator("fieldset")
      .filter({ has: page.locator("legend", { hasText: "Environment variables" }) });
    await environment.getByRole("button", { name: "Add value" }).click();
    await environment.getByLabel("Input name 1").fill("REGION");
    await environment.getByLabel("Value 1").fill("eu west");
    const bindings = form
      .locator("fieldset")
      .filter({ has: page.locator("legend", { hasText: "Secret bindings" }) });
    await bindings.getByRole("button", { name: "Add secret binding" }).click();
    await bindings.getByLabel("Input name 1").fill("API_TOKEN");
    await bindings.getByLabel("Secret 1", { exact: true }).click();
    await page.getByRole("option", { name: /MCP setup credential/ }).click();
    await assertNoDocumentHorizontalOverflow(page, "MCP setup inputs");
    const save = page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes" });
    await expect(save).toBeEnabled();
    if (mobile) await save.tap();
    else await save.click();
    await expect(form).toHaveCount(0);
    const saved = (await api.listMCPServers(workspaceId)).find((item) => item.id === server.id)!;
    expect(saved.configuration.args).toEqual(args);
    expect(saved.configuration.env).toEqual({ REGION: "eu west" });
    expect(saved.secret_bindings).toEqual([{ input_name: "API_TOKEN", secret_id: secret.id }]);
    expect(JSON.stringify(saved)).not.toContain("synthetic-mcp-input-secret");
  } finally {
    const current = (await api.listMCPServers(workspaceId)).find((item) => item.id === server.id);
    if (current) await api.deleteMCPServer(workspaceId, current.id, current.revision);
    await api.deleteSecret(secret.id, workspaceId);
  }
}

// @covers AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.4, AC-AGENTS-WORKSPACE-MCP-CONFIGURATION-006.5
export async function checkUnsupportedMCPArguments(
  page: Page,
  workspaceId: string,
  mobile: boolean,
) {
  await page.route("**/api/v1/mcp-marketplace*", (route) =>
    route.fulfill({
      json: {
        entries: [
          {
            name: "example/argument-inputs",
            title: "Argument setup",
            description: "Requires a directory input.",
            version: "1.0.0",
            status: "active",
            revision: 1,
            source: "registry",
            publisher_supplied: true,
            trust_notice: "Publisher-supplied metadata.",
            choices: [
              {
                id: "package-0",
                kind: "package",
                identifier: `@example/${"long-package-name".repeat(5)}`,
                version: "1.0.0",
                transport: "stdio",
                selectable: false,
                unsupported_reason: "input_arguments_unsupported",
              },
            ],
          },
        ],
        stale: false,
        degraded: false,
      },
    }),
  );
  await page.goto(`/settings/workspaces/${workspaceId}/mcp-servers`);
  const marketplace = page.getByRole("tab", { name: "Marketplace" });
  const review = page.getByRole("button", { name: "Review", exact: true });
  if (mobile) {
    await marketplace.tap();
    await review.tap();
  } else {
    await marketplace.click();
    await review.click();
  }
  const dialog = page.getByRole("dialog");
  await expect(dialog).toContainText(
    "This setup requests command arguments that cannot be configured yet. Choose another setup.",
  );
  await expect(dialog.getByRole("button", { name: "Add MCP server" })).toBeDisabled();
  await assertLocatorWithinViewportX(dialog, "unsupported MCP setup review");
  await assertNoDocumentHorizontalOverflow(page, "unsupported MCP setup review");
  expect(await dialog.evaluate((node) => node.scrollWidth <= node.clientWidth + 1)).toBe(true);
}
