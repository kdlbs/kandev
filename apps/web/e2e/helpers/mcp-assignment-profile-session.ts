import { PrAssetCapture } from "./pr-asset-capture";
import { waitForFiniteAnimations } from "./pr-capture";
import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";
import { openMCPComposerCustomization } from "./mcp-composer";

async function createDefinition(api: ApiClient, workspaceId: string, kind: string) {
  return api.createMCPServer(workspaceId, {
    runtime_name: `e2e-assignment-${kind}`,
    display_name: "Shared documentation",
    execution_mode: "remote",
    transport: "http",
    configuration: { url: "https://mcp.example.test/docs" },
  });
}
async function openChooser(page: Page, workspaceId: string, definitionId: string) {
  await page.goto(`/settings/workspaces/${workspaceId}/mcp-servers`);
  await page
    .getByTestId(`mcp-server-card-${definitionId}`)
    .getByRole("button", { name: "Use in", exact: true })
    .click();
  return page.getByTestId("mcp-assignment-chooser");
}

async function captureInlineProfile(page: Page, profileId: string, kind: string) {
  const picker = page.getByTestId(`profile-mcp-selection-${profileId}`);
  await expect(picker.getByRole("textbox", { name: "Search MCP servers" })).toHaveCount(1);
  await expect(picker.getByRole("button", { name: "MCP servers", exact: true })).toHaveCount(0);
  await expect(page.getByRole("link", { name: "Manage workspace catalog" })).toHaveCount(0);
  const capture = new PrAssetCapture(
    page,
    page.viewportSize()!.width < 768 ? "mobile-mcp-assignment.spec.ts" : "mcp-assignment.spec.ts",
    { captureKey: kind },
  );
  if (capture.capturing) {
    await waitForFiniteAnimations(picker);
    await capture.screenshot("inline", {
      caption: "Profile assignment highlights an unchecked server in the inline editor",
    });
    capture.flush();
  }
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.2, AC-AGENTS-MCP-ASSIGNMENT-002.3, AC-AGENTS-MCP-ASSIGNMENT-004.2
export async function checkAssignmentProfileHandoff(page: Page, api: ApiClient, seed: SeedData) {
  const { agents } = await api.listAgents();
  const agent = agents.find((item) =>
    item.profiles.some((profile) => profile.id === seed.agentProfileId),
  )!;
  const profile = agent.profiles.find((item) => item.id === seed.agentProfileId)!;
  const baseline = await api.getMCPSelections("profile", profile.id, seed.workspaceId);
  const definition = await createDefinition(api, seed.workspaceId, "profile");
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "PUT" && request.url().includes("mcp-selections"))
      writes.push(request.url());
  });
  try {
    const chooser = await openChooser(page, seed.workspaceId, definition.id);
    await chooser.getByRole("button", { name: "Agent profile", exact: true }).click();
    await chooser.getByRole("button", { name: profile.name, exact: true }).click();
    await expect(chooser).toContainText("Other workspaces keep their own choices");
    const viewport = page.viewportSize()!;
    for (const width of [320, 390, 767, 768, 900]) {
      await page.setViewportSize({ width, height: viewport.height });
      await expect(chooser).toContainText(profile.name);
      const open = chooser.getByRole("button", { name: "Open MCP settings", exact: true });
      await expect(open).toBeVisible();
      await waitForFiniteAnimations(chooser);
      const bounds = (await open.boundingBox())!;
      const coarsePointer = await page.evaluate(() => matchMedia("(pointer: coarse)").matches);
      if (width < 768 || coarsePointer) expect(bounds.height).toBeGreaterThanOrEqual(44);
      expect(bounds.x).toBeGreaterThanOrEqual(0);
      expect(bounds.x + bounds.width).toBeLessThanOrEqual(width);
      expect(bounds.y + bounds.height).toBeLessThanOrEqual(viewport.height);
      expect(await page.evaluate(() => document.documentElement.scrollWidth)).toBeLessThanOrEqual(
        width,
      );
    }
    await page.setViewportSize(viewport);
    await chooser.getByRole("button", { name: "Open MCP settings", exact: true }).click();
    const row = page.getByTestId(`mcp-choice-${definition.id}`);
    await expect(row).toBeVisible();
    await expect(row).toHaveAttribute("data-mcp-highlighted", "true");
    await expect(row.getByRole("checkbox")).not.toBeChecked();
    await captureInlineProfile(page, profile.id, "global-profile");
    expect(writes).toEqual([]);
    expect(
      (await api.getMCPSelections("profile", profile.id, seed.workspaceId)).definition_ids,
    ).toEqual(baseline.definition_ids);
    await row.getByRole("checkbox").click();
    expect(writes).toEqual([]);
    await page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("profile", profile.id, seed.workspaceId)).definition_ids,
      )
      .toEqual([...baseline.definition_ids, definition.id].sort());
    expect(writes).toHaveLength(1);
    await page.reload();
    expect(new URL(page.url()).searchParams.has("mcpServerId")).toBe(false);
    await expect(page.locator('[data-mcp-highlighted="true"]')).toHaveCount(0);
  } finally {
    await api.replaceMCPSelections(
      "profile",
      profile.id,
      seed.workspaceId,
      baseline.definition_ids,
    );
    await api.deleteMCPServer(seed.workspaceId, definition.id, definition.revision);
  }
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.4, AC-AGENTS-MCP-ASSIGNMENT-002.5, AC-AGENTS-MCP-ASSIGNMENT-004.2
export async function checkAssignmentSessionHandoff(page: Page, api: ApiClient, seed: SeedData) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "MCP assignment conversation",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing assignment session");
  await waitForSessionDone(api, task.id, task.session_id, "Assignment session fixture ready");
  const beforeSessions = (await api.listTaskSessions(task.id)).sessions;
  const baseline = await api.getMCPSelections("task_session", task.session_id, seed.workspaceId);
  const definition = await createDefinition(api, seed.workspaceId, "session");
  const capture = new PrAssetCapture(
    page,
    page.viewportSize()!.width < 768 ? "mobile-mcp-assignment.spec.ts" : "mcp-assignment.spec.ts",
    { captureKey: "session" },
  );
  const writes: string[] = [];
  page.on("request", (request) => {
    if (request.method() === "PUT" && request.url().includes("mcp-selections"))
      writes.push(request.url());
  });
  try {
    const chooser = await openChooser(page, seed.workspaceId, definition.id);
    await chooser.getByRole("button", { name: "Existing task session", exact: true }).click();
    await chooser
      .getByRole("textbox", { name: "Search owners" })
      .fill("MCP assignment conversation");
    await chooser.getByRole("button", { name: "MCP assignment conversation", exact: true }).click();
    // This task has one existing eligible session; choosing it cannot create a replacement.
    const { agents } = await api.listAgents();
    const profile = agents
      .flatMap((agent) => agent.profiles)
      .find((item) => item.id === seed.agentProfileId)!;
    await chooser.getByRole("button", { name: profile.name, exact: true }).click();
    await expect(chooser).toContainText(task.session_id);
    await expect(chooser).toContainText("future sessions keep their existing defaults");
    if (capture.capturing)
      await capture.screenshot("review", {
        caption: "Review the exact existing conversation and session identifier before assignment",
      });
    await chooser.getByRole("button", { name: "Open MCP settings", exact: true }).click();
    await new SessionPage(page).waitForLoad();
    expect(new URL(page.url()).searchParams.get("sessionId")).toBe(task.session_id);
    expect(new URL(page.url()).searchParams.get("mcpServerId")).toBe(definition.id);
    await expect(page.getByTestId("mcp-server-explorer")).not.toBeVisible();
    expect(writes).toEqual([]);
    expect(
      (await api.getMCPSelections("task_session", task.session_id, seed.workspaceId))
        .definition_ids,
    ).toEqual(baseline.definition_ids);
    const explorer = await openMCPComposerCustomization(page);
    const row = explorer.getByTestId(`mcp-choice-${definition.id}`);
    await expect(row).toHaveAttribute("data-mcp-highlighted", "true");
    await expect(row.getByRole("checkbox")).not.toBeChecked();
    await expect.poll(() => new URL(page.url()).searchParams.has("mcpServerId")).toBe(false);
    if (capture.capturing) {
      await waitForFiniteAnimations(explorer);
      await capture.screenshot("customize", {
        caption: "Existing session Customize highlights the saved server without selecting it",
      });
    }
    await row.getByRole("checkbox").click();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("task_session", task.session_id!, seed.workspaceId))
            .definition_ids,
      )
      .toEqual([...baseline.definition_ids, definition.id].sort());
    expect(writes).toHaveLength(1);
    expect((await api.listTaskSessions(task.id)).sessions.map((session) => session.id)).toEqual(
      beforeSessions.map((session) => session.id),
    );
  } finally {
    await api.replaceMCPSelections(
      "task_session",
      task.session_id,
      seed.workspaceId,
      baseline.definition_ids,
    );
    await api.deleteMCPServer(seed.workspaceId, definition.id, definition.revision);
    capture.flush();
  }
}

// @covers AC-AGENTS-MCP-ASSIGNMENT-002.2, AC-AGENTS-MCP-ASSIGNMENT-003.3
export async function checkAssignmentWorkspaceProfile(page: Page, api: ApiClient, seed: SeedData) {
  const response = await api.rawRequest(
    "POST",
    `/api/v1/office/workspaces/${seed.workspaceId}/agents`,
    {
      name: "Workspace assignment owner",
      role: "worker",
      agent_profile_id: seed.agentProfileId,
    },
  );
  const created = await response.json();
  expect(response.status, JSON.stringify(created)).toBe(201);
  const profileId = created.agent.id as string;
  const definition = await createDefinition(api, seed.workspaceId, "workspace-profile");
  try {
    expect(
      (await api.listAgents()).agents
        .flatMap((agent) => agent.profiles)
        .some((profile) => profile.id === profileId),
    ).toBe(false);
    const chooser = await openChooser(page, seed.workspaceId, definition.id);
    await chooser.getByRole("button", { name: "Agent profile", exact: true }).click();
    await chooser
      .getByRole("textbox", { name: "Search owners" })
      .fill("Workspace assignment owner");
    await chooser.getByRole("button", { name: "Workspace assignment owner", exact: true }).click();
    await chooser.getByRole("button", { name: "Open MCP settings", exact: true }).click();
    await expect(page.getByTestId("workspace-profile-mcp-destination")).toBeVisible();
    const row = page.getByTestId(`mcp-choice-${definition.id}`);
    await expect(row).toHaveAttribute("data-mcp-highlighted", "true");
    await expect(row.getByRole("checkbox")).not.toBeChecked();
    await captureInlineProfile(page, profileId, "workspace-profile");
    expect(
      (await api.getMCPSelections("profile", profileId, seed.workspaceId)).definition_ids,
    ).toEqual([]);
    await row.getByRole("checkbox").click();
    expect(
      (await api.getMCPSelections("profile", profileId, seed.workspaceId)).definition_ids,
    ).toEqual([]);
    await page
      .getByTestId("settings-floating-save")
      .getByRole("button", { name: "Save changes", exact: true })
      .click();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("profile", profileId, seed.workspaceId)).definition_ids,
      )
      .toEqual([definition.id]);
  } finally {
    await api.replaceMCPSelections("profile", profileId, seed.workspaceId, []);
    await api.deleteMCPServer(seed.workspaceId, definition.id, definition.revision);
    const deleted = await api.rawRequest("DELETE", `/api/v1/office/agents/${profileId}`);
    expect(deleted.ok).toBe(true);
  }
}
