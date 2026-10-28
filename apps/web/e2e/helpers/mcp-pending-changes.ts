import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type {
  MCPServerDefinition,
  MCPSessionApplyState,
  MCPSessionSelectionState,
} from "../../lib/types/http-mcp";
import { SessionPage } from "../pages/session-page";
import { openMCPComposerCustomization } from "./mcp-composer";
import { expectMCPCustomizationScrolling } from "./mcp-customization-scrolling";

export async function checkMCPPendingChanges(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const definitions: MCPServerDefinition[] = [];
  for (const [index, name] of [
    "Pending added server",
    "Pending removed server",
    "Pending updated server",
    ...Array.from({ length: 12 }, (_, i) => `Pending scroll ${i}`),
    "ZZ final additional server",
  ].entries()) {
    definitions.push(
      await api.createMCPServer(seed.workspaceId, {
        runtime_name: `pending-${index}`,
        display_name: name,
        execution_mode: "remote",
        transport: "sse",
        configuration: { url: `https://mcp.example.test/pending-${index}` },
        source: "custom",
      }),
    );
  }
  const [added, removed, updated] = definitions;
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "Exact pending MCP changes",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
      mcp_server_ids: [removed.id, updated.id],
    },
  );
  if (!task.session_id) throw new Error("Missing pending-change session");
  const sessionId = task.session_id;
  await page.goto(`/t/${task.id}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  // Real startup establishes the baseline. No browser-made accepted snapshot.
  await expect
    .poll(
      async () =>
        (await api.getMCPSelections("task_session", sessionId, seed.workspaceId)).mcp_state
          ?.comparison?.status,
    )
    .toBe("known");
  // A busy turn preserves the accepted setup while saved choices change.
  await session.sendMessageViaButton("/e2e:cancel-hold");
  await expect
    .poll(async () =>
      (await api.listTaskSessions(task.id)).sessions.some(
        (entry) => entry.id === sessionId && entry.state === "RUNNING",
      ),
    )
    .toBe(true);
  await api.replaceMCPSelections("task", task.id, seed.workspaceId, [updated.id]);
  await api.deleteMCPServer(seed.workspaceId, removed.id, removed.revision);
  definitions[2] = await api.updateMCPServer(seed.workspaceId, updated.id, {
    expected_revision: updated.revision,
    display_name: "Pending updated server renamed",
  });
  const saved = await api.replaceMCPSelections("task_session", sessionId, seed.workspaceId, [
    added.id,
  ]);
  const comparison = saved.mcp_state?.comparison;
  expect(comparison?.status).toBe("known");
  expect(comparison?.changes?.map((change) => change.action).sort()).toEqual([
    "add",
    "remove",
    "update",
  ]);
  expect(
    comparison?.changes?.find((change) => change.action === "remove")?.before?.display_name,
  ).toBe(removed.display_name);

  let timing: MCPSessionApplyState = "deferred_restart";
  let report: "normal" | "empty" | "unknown" | "stale" = "normal";
  let rejectSave = false;
  let writes = 0;
  const path = `**/api/v1/task-sessions/${sessionId}/mcp-selections*`;
  await page.route(path, async (route) => {
    if (route.request().method() === "PUT") {
      writes++;
      if (rejectSave)
        return route.fulfill({ status: 400, json: { error: "invalid MCP selection" } });
    }
    const response = await route.fetch();
    const body = await response.json();
    const state = body.mcp_state as MCPSessionSelectionState;
    state.apply_state = timing;
    if (report === "empty") state.comparison = { ...state.comparison!, changes: [] };
    if (report === "unknown") state.comparison = undefined;
    if (report === "stale")
      state.comparison = { ...state.comparison!, desired_revision: state.desired_revision - 1 };
    await route.fulfill({ response, json: body });
  });
  const reopen = async () => {
    await page.reload();
    await session.waitForLoad();
    return openMCPComposerCustomization(page);
  };
  try {
    const surface = await reopen();
    const body = surface.getByTestId("mcp-customization-scroll");
    const pending = body.getByTestId("mcp-pending-changes");
    await expect(pending).toContainText(added.display_name);
    await expect(pending).toContainText(removed.display_name);
    await expect(pending).toContainText("Pending updated server renamed");
    await expect(pending.getByRole("listitem")).toHaveCount(3);
    await expect(surface.getByTestId(`mcp-pending-row-${updated.id}`)).toContainText(
      "Update setup",
    );
    await expectMCPCustomizationScrolling(page, surface, mobile);
    const search = surface.getByRole("textbox", { name: "Search MCP servers" });
    await search.fill("Pending added");
    await expect(surface.getByRole("checkbox")).toHaveCount(1);
    await expect(surface.getByRole("checkbox")).toBeChecked();
    await expect(pending.getByRole("listitem")).toHaveCount(3);
    await pending.scrollIntoViewIfNeeded();
    await expect(pending).toBeVisible();
    await search.scrollIntoViewIfNeeded();
    rejectSave = true;
    await surface.getByRole("checkbox").click();
    await expect(surface.getByRole("alert")).toBeVisible();
    await expect(surface.getByRole("checkbox")).not.toBeChecked();
    await expect(surface.getByRole("checkbox")).toBeDisabled();
    await expect(pending.getByRole("listitem")).toHaveCount(3);
    expect(
      (await api.getMCPSelections("task_session", sessionId, seed.workspaceId)).definition_ids,
    ).toEqual([added.id]);
    rejectSave = false;
    await surface.getByRole("button", { name: "Reload choices", exact: true }).click();
    await expect(surface.getByRole("checkbox")).toBeEnabled();
    // The rejected draft remains unchecked; explicitly return to the saved choice first.
    await surface.getByRole("checkbox").click();
    await expect(surface.getByRole("checkbox")).toBeChecked();
    await expect(surface.getByRole("checkbox")).toBeEnabled();
    await surface.getByRole("checkbox").click();
    await expect(surface.getByRole("checkbox")).not.toBeChecked();
    await expect(pending.getByRole("listitem")).toHaveCount(2);
    await expect(pending).not.toContainText(added.display_name);
    await surface.getByRole("checkbox").click();
    await expect(surface.getByRole("checkbox")).toBeChecked();
    await expect(pending.getByRole("listitem")).toHaveCount(3);

    timing = "failed";
    await reopen();
    await expect(body.getByTestId("mcp-application-summary")).toContainText(
      "Changes could not be applied",
    );
    await expect(pending.getByRole("listitem")).toHaveCount(3);
    const previousWrites = writes;
    await body.getByRole("button", { name: "Retry MCP application" }).click();
    await expect.poll(() => writes).toBe(previousWrites + 1);
    await expect(pending.getByRole("listitem")).toHaveCount(3);

    timing = "pending_idle";
    await reopen();
    await expect(body.getByTestId("mcp-application-summary")).toContainText(
      "when the agent is idle",
    );
    await search.fill("Pending added");
    await expect(search).toBeFocused();
    const previousInput = await search.elementHandle();
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("task_session", sessionId, seed.workspaceId)).definition_ids,
      )
      .toEqual([added.id]);
    await expect(search).toHaveValue("Pending added");
    expect(await previousInput!.evaluate((node) => node.isConnected)).toBe(true);

    for (report of ["empty", "unknown", "stale"] as const) {
      await reopen();
      await expect(pending).toHaveCount(0);
      await expect(surface.getByTestId("mcp-pending-comparison-message")).toContainText(
        report === "empty"
          ? "matches the last accepted setup"
          : "individual changes cannot be listed yet",
      );
    }
    report = "normal";
    timing = "applied";
    await reopen();
    await expect(body.getByTestId("mcp-application-summary")).toContainText("Setup up to date");
    await expect(pending).toHaveCount(0);
    await expect(surface.locator('[data-testid^="mcp-pending-row-"]')).toHaveCount(0);
    await surface.getByTestId("mcp-explorer-close").click();
    await expect(page.getByTestId("mcp-status-trigger")).toBeFocused();

    // Return to real lifecycle evidence and watch idle acceptance clear the list.
    await page.unrouteAll({ behavior: "wait" });
    await session.cancelAgentButton().click();
    await session.waitForChatIdle({ timeout: 30_000, requireEditable: true });
    await openMCPComposerCustomization(page);
    await expect
      .poll(
        async () =>
          (await api.getMCPSelections("task_session", sessionId, seed.workspaceId)).mcp_state
            ?.apply_state,
        { timeout: 30_000 },
      )
      .toBe("applied");
    await expect(body.getByTestId("mcp-application-summary")).toContainText("Setup up to date");
    await expect(pending).toHaveCount(0);
    await expect(surface.locator('[data-testid^="mcp-pending-row-"]')).toHaveCount(0);
  } finally {
    await page.unrouteAll({ behavior: "wait" });
  }
}
