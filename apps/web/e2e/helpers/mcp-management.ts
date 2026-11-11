import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

// @covers AC-AGENTS-MCP-MANAGEMENT-001.1, AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-004.1
export async function checkMCPUsageNavigation(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const capture = new PrAssetCapture(
    page,
    mobile ? "mobile-mcp-management.spec.ts" : "mcp-management.spec.ts",
    { captureKey: `usage-${page.viewportSize()?.width}` },
  );
  const old = await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId);
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-management-usage",
    display_name: "Team documentation",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/docs" },
  });
  try {
    await api.replaceMCPSelections("repository", seed.repositoryId, seed.workspaceId, [
      ...old.definition_ids,
      server.id,
    ]);
    await page.goto(`/settings/workspaces/${seed.workspaceId}/mcp-servers`);
    const trigger = page
      .getByTestId(`mcp-server-card-${server.id}`)
      .getByRole("button", { name: "Used in", exact: true });
    expect((await trigger.boundingBox())!.height).toBeCloseTo(mobile ? 44 : 28, 2);
    await trigger.click();
    const usage = page.getByTestId("mcp-usage");
    await expect(usage).toHaveCount(1);
    await expect(usage).toBeVisible();
    const owner = usage.getByRole("link", { name: /E2E Repo/ });
    await expect(owner).toBeVisible();
    await waitForFiniteAnimations(usage);
    if (capture.capturing && (!mobile || page.viewportSize()?.width === 393)) {
      await capture.screenshot("usage", {
        caption: "Named MCP usage in the existing workspace catalog",
      });
    }
    if (!mobile) {
      await page.keyboard.press("Escape");
      await expect(trigger).toBeFocused();
      await trigger.click();
      await expect(owner).toBeVisible();
    }
    if (mobile) {
      expect(Math.round((await owner.boundingBox())!.height * 100) / 100).toBeGreaterThanOrEqual(
        44,
      );
      await assertNoDocumentHorizontalOverflow(page, "MCP usage");
    }
    await owner.click();
    await expect(page).toHaveURL(
      new RegExp(`/repositories\\?repositoryId=${seed.repositoryId}#mcp$`),
    );
    await expect(page.getByTestId("repository-mcp-selection-content")).toBeVisible();
    await expect(page.getByTestId(`mcp-choice-${server.id}`).getByRole("checkbox")).toBeChecked();
    await expect(page.getByTestId("settings-floating-save")).not.toBeVisible();
    await page.reload();
    await expect(page.getByTestId("repository-mcp-selection-content")).toBeVisible();
    await expect(page.getByTestId(`mcp-choice-${server.id}`).getByRole("checkbox")).toBeChecked();
    expect(
      (await api.getMCPSelections("repository", seed.repositoryId, seed.workspaceId))
        .definition_ids,
    ).toEqual([...old.definition_ids, server.id].sort());
  } finally {
    await api.replaceMCPSelections(
      "repository",
      seed.repositoryId,
      seed.workspaceId,
      old.definition_ids,
    );
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
    capture.flush();
  }
}

// @covers AC-AGENTS-MCP-MANAGEMENT-002.1, AC-AGENTS-MCP-MANAGEMENT-002.2, AC-AGENTS-MCP-MANAGEMENT-002.3, AC-AGENTS-MCP-MANAGEMENT-004.2
export async function checkGlobalProfileWorkspaceGuard(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const capture = new PrAssetCapture(
    page,
    mobile ? "mobile-mcp-management.spec.ts" : "mcp-management.spec.ts",
    { captureKey: "profile" },
  );
  const { agents } = await api.listAgents();
  const workspaceName = (await api.listWorkspaces()).workspaces.find(
    (w) => w.id === seed.workspaceId,
  )!.name;
  const agent = agents.find((item) =>
    item.profiles.some((profile) => profile.id === seed.agentProfileId),
  )!;
  const other = await api.createWorkspace("MCP context B");
  const baseline = await api.getMCPSelections("profile", seed.agentProfileId, seed.workspaceId);
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-management-profile",
    display_name: "Workspace A tools",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/profile" },
  });
  const route = `/settings/agents/${agent.id}/profiles/${seed.agentProfileId}`;
  try {
    await page.goto(route);
    const workspace = page.locator(`#mcp-workspace-${seed.agentProfileId}`);
    await expect(
      page.getByText("Select a workspace before configuring MCP servers.", { exact: true }),
    ).toBeVisible();
    await expect(
      page.getByRole("button", { name: "Global profile legacy configuration", exact: true }),
    ).toHaveCount(0);
    await workspace.click();
    await page.getByRole("option", { name: workspaceName, exact: true }).click();
    const picker = page.getByTestId(`profile-mcp-selection-${seed.agentProfileId}`);
    const choice = picker.getByTestId(`mcp-choice-${server.id}`).getByRole("checkbox");
    await expect(choice).toBeVisible();
    await expect(picker.getByRole("textbox", { name: "Search MCP servers" })).toHaveCount(1);
    await expect(page.getByRole("link", { name: "Manage workspace catalog" })).toHaveCount(0);
    await expect(
      page.locator("#mcp").getByText("Choose MCP servers for this profile.", { exact: true }),
    ).toHaveCount(1);
    await choice.click();
    const viewport = page.viewportSize()!;
    for (const width of [320, 768, viewport.width]) {
      await page.setViewportSize({ width, height: viewport.height });
      await expect(choice).toBeChecked();
      await assertNoDocumentHorizontalOverflow(page, "MCP profile draft across breakpoints");
    }
    await page.keyboard.press("Escape");
    await workspace.click();
    await page.getByRole("option", { name: other.name, exact: true }).click();
    const guard = page.getByRole("alertdialog");
    await expect(guard).toBeVisible();
    await guard.getByRole("button", { name: "Continue editing", exact: true }).click();
    await expect(workspace).toContainText(workspaceName);
    expect(
      (await api.getMCPSelections("profile", seed.agentProfileId, seed.workspaceId)).definition_ids,
    ).toEqual(baseline.definition_ids);
    await workspace.click();
    await page.getByRole("option", { name: other.name, exact: true }).click();
    let rejectSave = true;
    await page.route(
      `**/api/v1/agent-profiles/${seed.agentProfileId}/mcp-selections`,
      async (route) => {
        if (route.request().method() === "PUT" && rejectSave) {
          rejectSave = false;
          await route.fulfill({
            status: 500,
            contentType: "application/json",
            body: JSON.stringify({ error: "failed to save selections" }),
          });
        } else await route.continue();
      },
    );
    await guard.getByRole("button", { name: "Save and leave", exact: true }).click();
    await expect(guard).toBeVisible();
    await expect(workspace).toContainText(workspaceName);
    await expect(
      page.getByText("Could not save the MCP selection.", { exact: true }),
    ).toBeVisible();
    await expect(guard.getByRole("button", { name: "Save and leave", exact: true })).toBeDisabled();
    await guard.getByRole("button", { name: "Continue editing", exact: true }).click();
    await page.getByRole("button", { name: "Reload choices", exact: true }).click();
    await workspace.click();
    await page.getByRole("option", { name: other.name, exact: true }).click();
    await guard.getByRole("button", { name: "Save and leave", exact: true }).click();
    await expect(workspace).toContainText(other.name);
    expect(
      (await api.getMCPSelections("profile", seed.agentProfileId, seed.workspaceId)).definition_ids,
    ).toContain(server.id);
    await page.reload();
    await expect(workspace).toContainText(other.name);
    await page.goto(`${route}?workspaceId=${seed.workspaceId}#mcp`);
    await expect(workspace).toContainText(workspaceName);
    if (capture.capturing) {
      await waitForFiniteAnimations(page.locator("#mcp"));
      await capture.screenshot("profile", {
        caption: "Workspace-aware global profile MCP choices",
      });
    }
    await expect(choice).toBeChecked();
    await choice.click();
    await page.keyboard.press("Escape");
    await workspace.click();
    await page.getByRole("option", { name: other.name, exact: true }).click();
    await guard.getByRole("button", { name: "Discard and leave", exact: true }).click();
    await expect(workspace).toContainText(other.name);
    expect(
      (await api.getMCPSelections("profile", seed.agentProfileId, seed.workspaceId)).definition_ids,
    ).toContain(server.id);
    if (mobile) {
      expect((await workspace.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(page, "global MCP profile");
    }
    await page.goto(`${route}?workspaceId=missing&workspaceId=${seed.workspaceId}#mcp`);
    await expect(page.getByRole("alert")).toContainText("destination is unavailable");
    await expect(page.getByTestId(`profile-mcp-selection-${seed.agentProfileId}`)).toHaveCount(0);
  } finally {
    await api.replaceMCPSelections(
      "profile",
      seed.agentProfileId,
      seed.workspaceId,
      baseline.definition_ids,
    );
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
    await api.deleteWorkspace(other.id, other.name);
    capture.flush();
  }
}

// @covers AC-AGENTS-MCP-MANAGEMENT-001.3, AC-AGENTS-MCP-MANAGEMENT-001.4, AC-AGENTS-MCP-MANAGEMENT-004.1
export async function checkMCPUsagePaging(page: Page, api: ApiClient, seed: SeedData) {
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "e2e-management-pages",
    display_name: "Paged usage",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/pages" },
  });
  const items = Array.from({ length: 30 }, (_, index) => ({
    scope: "repository",
    owner_id: `owner-${index}`,
    workspace_id: seed.workspaceId,
    display_name: `Repository ${index}: long documentation and infrastructure configuration`,
    owner_state: "available",
    destination: { kind: "repository", workspace_id: seed.workspaceId, owner_id: `owner-${index}` },
  }));
  let mode: "sparse" | "failure" | "rows" | "denied" | "delayed" = "sparse";
  const retryPage = Promise.withResolvers<void>();
  await page.route(
    `**/api/v1/workspaces/${seed.workspaceId}/mcp-servers/${server.id}/usage**`,
    async (route) => {
      if (mode === "delayed") await retryPage.promise;
      const status = { failure: 500, denied: 404, sparse: 200, rows: 200, delayed: 200 }[mode];
      const body =
        status === 200
          ? {
              workspace_id: seed.workspaceId,
              definition_id: server.id,
              items: mode === "rows" || mode === "delayed" ? items : [],
              ...(mode === "sparse" ? { next_cursor: "opaque-next" } : {}),
            }
          : { error: "unavailable" };
      await route.fulfill({ status, contentType: "application/json", body: JSON.stringify(body) });
    },
  );
  try {
    await page.goto(
      `/settings/workspaces/${seed.workspaceId}/mcp-servers?serverId=${server.id}&view=usage`,
    );
    const usage = page.getByTestId("mcp-usage");
    const more = usage.getByRole("button", { name: "Load more", exact: true });
    await expect(more).toBeEnabled();
    await expect(usage.getByText("No saved selections are visible to you.")).toHaveCount(0);
    mode = "failure";
    await more.click();
    await expect(usage.getByRole("alert")).toBeVisible();
    mode = "rows";
    await usage.getByRole("button", { name: "Retry", exact: true }).click();
    await expect(usage.getByRole("link")).toHaveCount(30);
    await usage.getByRole("link").last().scrollIntoViewIfNeeded();
    await expect(usage.getByRole("link").last()).toBeInViewport();
    await assertNoDocumentHorizontalOverflow(page, "paged MCP owners");
    mode = "failure";
    const viewport = page.viewportSize()!;
    for (const width of [393, 900, viewport.width]) {
      await page.setViewportSize({ width, height: viewport.height });
      await expect(usage.getByRole("link")).toHaveCount(30);
    }
    await usage.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(usage.getByRole("alert")).toContainText("out of date");
    await expect(usage.getByRole("link")).toHaveCount(0);
    mode = "delayed";
    const retry = usage.getByRole("button", { name: "Retry", exact: true });
    await retry.click();
    await expect(retry).toBeDisabled();
    await expect(usage.getByRole("link")).toHaveCount(0);
    retryPage.resolve();
    await expect(usage.getByRole("link")).toHaveCount(30);
    mode = "denied";
    await usage.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(usage.getByRole("alert")).toContainText("destination is unavailable");
    await expect(usage.getByText(items[0].display_name, { exact: true })).toHaveCount(0);
  } finally {
    retryPage.resolve();
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
  }
}
