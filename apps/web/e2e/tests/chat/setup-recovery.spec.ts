import { expect } from "@playwright/test";
import { test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  createMcpRecoveryFixture,
  destroyMcpRecoveryTerminals,
  E2E_MCP_SERVER_ID,
  installAgentMcpRecoveryRoutes,
} from "../../helpers/agent-mcp-preparation";

import { openQuickChatSetup, selectAgentIfNeeded } from "./quick-chat-helpers";

test.describe("Setup recovery UX (desktop)", () => {
  test("shows authentication warning with amber styling and recovery actions", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const fixture = await createMcpRecoveryFixture(
      apiClient,
      seedData,
      "Agent MCP Warning Desktop",
    );
    await installAgentMcpRecoveryRoutes(testPage, {
      sessionId: fixture.sessionId,
      taskEnvironmentId: fixture.taskEnvironmentId,
      terminalId: fixture.authenticationTerminalId,
    });

    try {
      await testPage.goto(`/t/${fixture.taskId}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const preparation = testPage.getByTestId("prepare-progress-panel");
      await expect(preparation).toHaveAttribute("data-status", "completed_with_warnings");
      await expect(preparation).toHaveAttribute("data-expanded", "true");

      await expect(preparation).toContainText(`Verify connection: ${E2E_MCP_SERVER_ID}`);
      await expect(preparation.getByTestId("prepare-step-warning-icon")).toBeVisible();

      const actions = preparation.getByTestId("agent-mcp-recovery-actions");
      await expect(actions).toBeVisible();
      const authMessage = actions.getByText("Authentication is required.");
      await expect(authMessage).toBeVisible();
      await expect(authMessage).toHaveClass(/text-amber-700/);

      await expect(actions.getByTestId("agent-mcp-authenticate")).toBeVisible();
      await expect(actions.getByTestId("agent-mcp-retry")).toBeVisible();
    } finally {
      await destroyMcpRecoveryTerminals(apiClient, fixture);
    }
  });

  test("displays inline error on pre-session setup failure", async ({ testPage }) => {
    await testPage.route("**/quick-chat*", async (route) => {
      if (route.request().method() !== "POST") {
        await route.continue();
        return;
      }
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "Invalid repository configuration" }),
      });
    });

    const dialog = await openQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage);
    const openingPrompt = "Keep this request available for retry.";
    const prompt = dialog.getByTestId("task-description-input");
    await prompt.fill(openingPrompt);
    const send = dialog.getByTestId("quick-chat-send");
    await expect(send).toBeEnabled({ timeout: 10_000 });

    await send.click();

    const setupError = dialog.getByTestId("quick-chat-setup-error");
    await expect(setupError).toBeVisible({ timeout: 10_000 });
    await expect(setupError).toContainText("Invalid repository configuration");
    await expect(prompt).toHaveValue(openingPrompt);
    await expect(send).toBeEnabled();
  });

  test("retains a real quick chat without changing selection after a delayed response", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(90_000);
    const accepted = Promise.withResolvers<{ task_id: string; session_id: string }>();
    const release = Promise.withResolvers<void>();
    const deleted: string[] = [];
    testPage.on("request", (request) => {
      if (request.method() === "DELETE") deleted.push(request.url());
    });
    await testPage.route("**/quick-chat", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const response = await route.fetch();
      expect(response.ok()).toBe(true);
      accepted.resolve(await response.json());
      await release.promise;
      await route.fulfill({ response });
    });
    const dialog = await openQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage);
    await dialog.getByTestId("task-description-input").fill("Prepare a short implementation plan.");
    await dialog.getByTestId("quick-chat-send").click();
    const created = await accepted.promise;
    try {
      // The real backend announces the persisted session before HTTP completes.
      await expect(
        dialog.locator(`[data-tab-reference="conversation:${created.session_id}"]`),
      ).toHaveCount(1);
      await dialog.getByTestId("quick-chat-add-menu-trigger").click();
      await testPage.getByTestId("quick-chat-new-agent").click();
      const response = testPage.waitForResponse(
        (r) => r.url().endsWith("/quick-chat") && r.request().method() === "POST",
      );
      release.resolve();
      await response;
      await expect(dialog.getByTestId("quick-chat-setup")).toBeVisible();
      await expect(
        dialog.locator(`[data-tab-reference="conversation:${created.session_id}"]`),
      ).toHaveCount(1);
      expect(deleted.some((url) => url.endsWith(`/tasks/${created.task_id}`))).toBe(false);
      expect(
        (await apiClient.listTaskSessions(created.task_id)).sessions.some(
          (row) => row.id === created.session_id,
        ),
      ).toBe(true);
      await testPage.reload();
      const modifier = process.platform === "darwin" ? "Meta" : "Control";
      await testPage.keyboard.press(`${modifier}+Shift+q`);
      await expect(
        testPage
          .getByRole("dialog", { name: "Quick Chat" })
          .locator(`[data-tab-reference="conversation:${created.session_id}"]`),
      ).toHaveCount(1);
    } finally {
      release.resolve();
    }
  });

  test("retains a persisted setup error and its recovery controls after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const failure = "The agent could not start.";
    let created!: { task_id: string; session_id: string };
    await testPage.route("**/quick-chat", async (route) => {
      if (route.request().method() !== "POST") return route.continue();
      const response = await route.fetch();
      expect(response.ok()).toBe(true);
      created = await response.json();
      await expect
        .poll(
          async () =>
            (await apiClient.listTaskSessions(created.task_id)).sessions.find(
              (row) => row.id === created.session_id,
            )?.state,
          { timeout: 30_000 },
        )
        .toMatch(/WAITING_FOR_INPUT|COMPLETED/);
      const occurredAt = new Date().toISOString();
      await apiClient.seedTaskSession(created.task_id, {
        state: "FAILED",
        completedAt: occurredAt,
        sessionId: created.session_id,
        agentProfileId: seedData.agentProfileId,
        errorMessage: failure,
        metadata: {
          last_agent_error: {
            message: failure,
            occurred_at: occurredAt,
            code: "generic_launch_failure",
            phase: "bootstrap",
            scope: "session",
            stamp: "retained-setup-e2e",
          },
        },
      });
      await apiClient.seedSessionMessage(created.session_id, {
        type: "status",
        content: failure,
        createdAt: occurredAt,
        metadata: {
          recovery_actions: true,
          scope: "session",
          error_stamp: "retained-setup-e2e",
          actions: [
            {
              type: "ws_request",
              label: "Resume session",
              icon: "refresh",
              test_id: "recovery-resume-button",
              params: {
                method: "session.recover",
                payload: {
                  task_id: created.task_id,
                  session_id: created.session_id,
                  action: "resume",
                },
              },
            },
          ],
        },
      });
      await route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: "failed to start session", ...created }),
      });
    });
    const dialog = await openQuickChatSetup(testPage);
    await selectAgentIfNeeded(dialog, testPage);
    await dialog.getByTestId("task-description-input").fill("Keep this request for recovery.");
    const retainedResponse = testPage.waitForResponse(
      (response) =>
        response.url().endsWith("/quick-chat") && response.request().method() === "POST",
    );
    await dialog.getByTestId("quick-chat-send").click();
    expect((await retainedResponse).status()).toBe(500);
    await expect(
      dialog.getByTestId("session-recovery-action-message").getByText(failure, { exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByTestId("session-recovery-action-message").getByTestId("recovery-resume-button"),
    ).toBeVisible();
    await testPage.reload();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+Shift+q`);
    await expect(
      dialog.getByTestId("session-recovery-action-message").getByText(failure, { exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByTestId("session-recovery-action-message").getByTestId("recovery-resume-button"),
    ).toBeVisible();
    const sessions = (await apiClient.listTaskSessions(created.task_id)).sessions;
    expect(sessions).toHaveLength(1);
    expect(sessions[0].id).toBe(created.session_id);
    expect(sessions[0].state).toBe("FAILED");
  });
});
