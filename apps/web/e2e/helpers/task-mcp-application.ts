import { expect, type Page, type TestInfo } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { SeedData } from "../fixtures/test-base";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";
import { SessionPage } from "../pages/session-page";
import { activate, builtinView, chooseBuiltin } from "./builtin-mcp-management";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

// Native provider continuity is proved separately by the pinned backend conformance test.
// This test drives saved intent through the real backend and injects later receipt views.
export async function checkTaskMCPApplication(
  page: Page,
  api: ApiClient,
  seed: SeedData,
  touch: boolean,
  info: TestInfo,
) {
  const task = await api.createTaskWithAgent(
    seed.workspaceId,
    "MCP application status",
    seed.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await page.goto(`/t/${task.id}`);
  const sessionPage = new SessionPage(page);
  await sessionPage.waitForLoad();
  await sessionPage.waitForChatIdle({ timeout: 30_000 });
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  const explorer = page.getByTestId("mcp-server-explorer");
  await activate(explorer.getByTestId("mcp-customize-tab"), touch);
  await activate(explorer.getByTestId("builtin-mcp-package-configuration"), touch);
  await chooseBuiltin(page, "configuration", "On", touch);
  await activate(explorer.getByTestId("builtin-mcp-save"), touch);
  await expect
    .poll(
      async () => (await builtinView(api, "tasks", task.id)).preferences?.packages.configuration,
    )
    .toBe("on");
  const feedback = explorer.getByTestId("builtin-mcp-application");
  await expect(feedback).toHaveAttribute("data-state", "pending");
  await expect(feedback.getByRole("status")).toContainText("Saved");
  const { sessions } = await api.listTaskSessions(task.id);
  const session = sessions.find((s) => s.is_primary)!;
  const response = await api.rawRequest("GET", `/api/v1/task-sessions/${session.id}/mcp-built-in`);
  const saved: BuiltinMCPSessionView = await response.json();
  expect(saved.application).toBeDefined();
  let current = applicationView(saved, "failed");
  let retries = 0;
  await page.route(`**/api/v1/task-sessions/${session.id}/mcp-built-in`, (route) =>
    route.fulfill({ json: current }),
  );
  await page.route(`**/api/v1/task-sessions/${session.id}/mcp-built-in/retry`, async (route) => {
    expect(route.request().method()).toBe("POST");
    expect(route.request().postDataJSON()).toEqual({
      version: 1,
      request_id: saved.application!.request_id,
      expected_row_version: current.application!.row_version,
    });
    retries++;
    current = applicationView(saved, "verifying");
    await route.fulfill({ json: current });
  });
  // Reopening also proves that status comes from saved evidence rather than local save state.
  await page.keyboard.press("Escape");
  await activate(page.getByTestId("mcp-status-trigger"), touch);
  await expect(feedback).toHaveAttribute("data-state", "failed");
  const retry = explorer.getByTestId("builtin-mcp-application-retry");
  if (touch) {
    for (const width of [320, 393, 767, 768, 900]) {
      await page.setViewportSize({ width, height: 852 });
      if (!(await explorer.isVisible()))
        await activate(page.getByTestId("mcp-status-trigger"), touch);
      await expect(retry).toBeVisible();
      expect((await retry.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(page, `MCP application Retry ${width}`);
    }
    await page.setViewportSize({ width: 393, height: 852 });
    if (!(await explorer.isVisible()))
      await activate(page.getByTestId("mcp-status-trigger"), touch);
    await expect(retry).toBeVisible();
  }
  await explorer.screenshot({ path: info.outputPath("mcp-application-failed.png") });
  await activate(retry, touch);
  await expect(feedback).toHaveAttribute("data-state", "applying");
  expect(retries).toBe(1);
  current = applicationView(saved, "applied");
  await expect(feedback).toHaveAttribute("data-state", "applied");
  await expect(feedback).toContainText("saved settings");
  await expect(retry).toHaveCount(0);
  await activate(explorer.getByTestId("mcp-connections-tab"), touch);
  const configuration = explorer.getByRole("button", { name: /kandev-configuration/ });
  await activate(configuration, touch);
  await expect(explorer).toContainText("list_executors_kandev");
  await assertNoDocumentHorizontalOverflow(page, "MCP application applied catalog");
  await explorer.screenshot({ path: info.outputPath("mcp-application-applied.png") });
  expect((await builtinView(api, "tasks", task.id)).preferences?.packages.configuration).toBe("on");
}
function applicationView(
  saved: BuiltinMCPSessionView,
  stage: "failed" | "verifying" | "applied",
): BuiltinMCPSessionView {
  const applied = stage === "applied";
  const manifest = saved.desired!;
  const catalog = Object.fromEntries(
    manifest.packages.map((id) => [
      id === "core" ? "kandev" : `kandev-${id}`,
      [id === "configuration" ? "list_executors_kandev" : "step_complete_kandev"],
    ]),
  );
  return {
    ...saved,
    provider_available: applied,
    can_retry_application: stage === "failed",
    application: { ...saved.application!, stage },
    provider_binding: applied
      ? {
          session_id: saved.session_id,
          manifest,
          catalog,
          accepted_at: "2026-11-16T21:00:00Z",
          scope: {
            binding_id: "receipt-current",
            execution_id: saved.execution_id!,
            helper_id: "helper",
            process_generation: 2,
            harness_generation: 1,
            profile_digest: "a".repeat(64),
          },
        }
      : undefined,
  };
}
