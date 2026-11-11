import { expect, type Page, type Request } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import {
  assertNoDocumentHorizontalOverflow,
  assertLocatorWithinViewportX,
} from "./layout-assertions";
import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";

// @covers AC-AGENTS-MCP-ASSIGNMENT-001.4, AC-AGENTS-MCP-ASSIGNMENT-002.1, AC-AGENTS-MCP-ASSIGNMENT-002.5, AC-AGENTS-MCP-ASSIGNMENT-004.1
export async function checkAssignmentRepositoryHandoff(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  phone: boolean,
) {
  const baseline = await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId);
  const definition = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-assignment-handoff",
    display_name: "Team documentation",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/docs" },
  });
  const capture = new PrAssetCapture(
    page,
    phone ? "mobile-mcp-assignment.spec.ts" : "mcp-assignment.spec.ts",
    { captureKey: "repository-handoff" },
  );
  const writes: string[] = [];
  const listener = (request: Request) => {
    const url = new URL(request.url());
    if (
      url.pathname.startsWith("/api/") &&
      !["GET", "HEAD", "OPTIONS"].includes(request.method()) &&
      !url.pathname.endsWith("/sidebar/query")
    )
      writes.push(`${request.method()} ${url.pathname}`);
  };
  try {
    await page.goto(`/settings/workspaces/${seed.workspaceId}/mcp-servers`);
    page.on("request", listener);
    const useIn = page
      .getByTestId(`mcp-server-card-${definition.id}`)
      .getByRole("button", { name: "Use in", exact: true });
    await expect(useIn).toBeVisible();
    await useIn.click();
    const chooser = page.getByTestId("mcp-assignment-chooser");
    await expect(chooser).toBeVisible();
    await chooser.getByRole("button", { name: "Repository", exact: true }).click();
    await chooser.getByRole("textbox", { name: "Search owners" }).fill("E2E Repo");
    await chooser.getByRole("button", { name: "E2E Repo", exact: true }).click();
    await expect(chooser).toContainText("future");
    await waitForFiniteAnimations(chooser);
    await assertLocatorWithinViewportX(chooser, "MCP assignment chooser");
    await assertNoDocumentHorizontalOverflow(page, "MCP assignment chooser");
    const open = chooser.getByRole("button", { name: "Open MCP settings", exact: true });
    if (phone) expect((await open.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    if (capture.capturing)
      await capture.screenshot("review", {
        caption: "Review repository scope before opening MCP settings",
      });
    expect(writes).toEqual([]);
    let release!: () => void;
    const gate = new Promise<void>((resolve) => {
      release = resolve;
    });
    const catalog = `**/api/v1/workspaces/${seed.workspaceId}/mcp-servers`;
    await page.route(
      catalog,
      async (route) => {
        await gate;
        await route.continue();
      },
      { times: 1 },
    );
    try {
      await open.click();
      await expect(open).toHaveAccessibleName("Open MCP settings");
      await expect(open).toBeDisabled();
      await expect(chooser.getByRole("status")).toHaveText("Loading…");
      if (capture.capturing)
        await capture.screenshot("checking", {
          caption: "Destination checking is announced while Open MCP settings keeps its name",
        });
    } finally {
      release();
    }
    const row = page.getByTestId(`mcp-choice-${definition.id}`);
    await expect(row).toBeVisible();
    await expect(row).toHaveAttribute("data-mcp-highlighted", "true");
    await expect(row.getByRole("checkbox")).not.toBeChecked();
    await expect(page).toHaveURL(
      new RegExp(`/repositories\\?repositoryId=${seed.repositoryId}#mcp$`),
    );
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).toEqual(baseline.definition_ids);
    expect(writes).toEqual([]);
    await row.getByRole("checkbox").click();
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).toEqual(baseline.definition_ids);
    if (capture.capturing)
      await capture.screenshot("settings", {
        caption: "Repository choices remain a draft until Save changes",
      });
    await page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
            .definition_ids,
      )
      .toEqual([...baseline.definition_ids, definition.id].sort());
    await expect(page.getByTestId("settings-floating-save")).not.toBeVisible();
    expect(writes.filter((write) => write.endsWith("/mcp-selections"))).toEqual([
      `PUT /api/v1/repositories/${seed.repositoryId}/mcp-selections`,
    ]);
    // The shared save can also persist the repository form after its MCP contributor.
    expect(
      writes.every((write) =>
        [
          `PUT /api/v1/repositories/${seed.repositoryId}/mcp-selections`,
          `PATCH /api/v1/repositories/${seed.repositoryId}`,
        ].includes(write),
      ),
    ).toBe(true);
    expect(
      (await api.listMCPServers(seed.workspaceId)).filter((server) => server.id === definition.id),
    ).toHaveLength(1);
  } finally {
    page.off("request", listener);
    await api.replaceMCPSelections(
      "repository",
      seed.repositoryId,
      seed.workspaceId,
      baseline.definition_ids,
    );
    await api.deleteMCPServer(seed.workspaceId, definition.id, definition.revision);
    capture.flush();
  }
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-003.1, AC-AGENTS-MCP-ASSIGNMENT-003.2, AC-AGENTS-MCP-ASSIGNMENT-003.4
export async function checkAssignmentRecovery(page: Page, api: ApiClient, seed: SeedData) {
  const definition = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-assignment-recovery",
    display_name: "Saved recovery tools",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/docs" },
  });
  let revision = definition.revision;
  const endpoint = `**/api/v1/workspaces/${seed.workspaceId}/repositories*`;
  let failRead = true;
  try {
    await page.goto(`/settings/workspaces/${seed.workspaceId}/mcp-servers`);
    await page
      .getByTestId(`mcp-server-card-${definition.id}`)
      .getByRole("button", { name: "Use in", exact: true })
      .click();
    await page.route(endpoint, async (route) => {
      if (failRead && route.request().method() === "GET")
        return route.fulfill({ status: 503, json: { error: "unavailable" } });
      await route.continue();
    });
    const chooser = page.getByTestId("mcp-assignment-chooser");
    await chooser.getByRole("button", { name: "Repository", exact: true }).click();
    await expect(chooser.getByRole("alert")).toContainText("The server remains saved");
    failRead = false;
    await chooser.getByRole("button", { name: "Retry", exact: true }).click();
    await chooser.getByRole("button", { name: "E2E Repo", exact: true }).click();
    const updated = await api.updateMCPServer(seed.workspaceId, definition.id, {
      enabled: false,
      expected_revision: revision,
    });
    revision = updated.revision;
    await chooser.getByRole("button", { name: "Open MCP settings", exact: true }).click();
    await expect(chooser.getByRole("alert")).toContainText("no longer available");
    expect(
      (await api.listMCPServers(seed.workspaceId)).filter((item) => item.id === definition.id),
    ).toHaveLength(1);
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).not.toContain(definition.id);
    await page.keyboard.press("Escape");
    await page.reload();
    const card = page.getByTestId(`mcp-server-card-${definition.id}`);
    await expect(card.getByRole("button", { name: "Use in", exact: true })).toBeDisabled();
    await expect(card).toContainText("Enable this server");
  } finally {
    await page.unroute(endpoint);
    await api.deleteMCPServer(seed.workspaceId, definition.id, revision);
  }
}
