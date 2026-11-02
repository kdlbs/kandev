import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import { PrAssetCapture } from "./pr-asset-capture";

// @covers AC-AGENTS-MCP-MANAGEMENT-001.2, AC-AGENTS-MCP-MANAGEMENT-004.1
export async function checkMCPSessionAdmission(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  mobile: boolean,
) {
  const task = await api.createTask(seed.workspaceId, "Review exact session navigation", {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  const { session_id } = await api.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seed.agentProfileId,
  });
  const server = await api.createMCPServer(seed.workspaceId, {
    runtime_name: "session-admission-tools",
    display_name: "Session navigation tools",
    execution_mode: "remote",
    transport: "sse",
    configuration: { url: "https://mcp.example.test/navigation" },
  });
  await api.replaceMCPSelections("task_session", session_id, seed.workspaceId, [server.id]);
  const capture = new PrAssetCapture(page, "mcp-session-admission.spec.ts", {
    captureKey: mobile ? "phone" : "desktop",
  });
  let failed = true;
  await page.route(`**/api/v1/tasks/${task.id}/sessions*`, async (route) => {
    if (failed)
      await route.fulfill({
        status: 503,
        contentType: "application/json",
        body: JSON.stringify({ error: "temporary session read failure" }),
      });
    else await route.continue();
  });
  try {
    await page.goto(
      `/settings/workspaces/${seed.workspaceId}/mcp-servers?serverId=${server.id}&view=usage`,
    );
    await page.getByTestId("mcp-usage").locator(`a[href*="sessionId=${session_id}"]`).click();
    const notice = page.getByTestId("task-session-unavailable");
    await expect(notice.getByRole("button", { name: "Retry", exact: true })).toBeVisible();
    if (mobile) {
      await page.setViewportSize({ width: 320, height: 852 });
      const retry = notice.getByRole("button", { name: "Retry", exact: true });
      expect((await retry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      expect(
        await retry.evaluate((element) => {
          const rect = element.getBoundingClientRect();
          return element.contains(
            document.elementFromPoint(rect.x + rect.width / 2, rect.y + rect.height / 2),
          );
        }),
      ).toBe(true);
    }

    if (capture.capturing)
      await capture.screenshot("retry", {
        caption: `${mobile ? "Phone" : "Desktop"} recoverable exact-session read failure`,
      });
    failed = false;
    await notice.getByRole("button", { name: "Retry", exact: true }).click();
    await expect(notice).toHaveCount(0);
    await expect(page.getByTestId("session-chat")).toBeVisible();
    await expect(page).toHaveURL(new RegExp(`sessionId=${session_id}`));
    await page.goto(`/t/${task.id}?sessionId=missing-session`);
    await expect(notice.getByRole("alert")).toContainText("Session unavailable");
    await expect(notice.getByRole("button")).toHaveCount(0);
    if (capture.capturing)
      await capture.screenshot("missing", {
        caption: `${mobile ? "Phone" : "Desktop"} unavailable exact session without conversation fallback`,
      });
    expect(await page.evaluate(() => document.documentElement.scrollWidth <= innerWidth)).toBe(
      true,
    );
  } finally {
    failed = false;
    await api.replaceMCPSelections("task_session", session_id, seed.workspaceId, []);
    await api.deleteMCPServer(seed.workspaceId, server.id, server.revision);
    await api.deleteTask(task.id);
    capture.flush();
  }
}
