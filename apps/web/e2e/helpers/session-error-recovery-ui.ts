import type { Page } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import { expect, test } from "../fixtures/test-base";
import { waitForSessionDone } from "./session";
import { SessionPage } from "../pages/session-page";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function sessionErrorDetailsScenario() {
  test("details wrap and copy safely", async ({ testPage, apiClient, seedData }, testInfo) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Recovery diagnostic layout",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("fixture session missing");
    await waitForSessionDone(apiClient, task.id, task.session_id, "diagnostic fixture settled");
    const stamp = "diagnostic-layout-fixture";
    const occurredAt = new Date().toISOString();
    const diagnostics =
      "Authorization: Bearer synthetic-private-value\n" +
      "Resume: connection refused. 恢復失敗。\n" +
      "Long diagnostic words ".repeat(1000);
    await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      errorMessage: "Session could not resume",
      sessionId: task.session_id,
      agentProfileId: seedData.agentProfileId,
      metadata: {
        last_agent_error: {
          message: "Session could not resume",
          stamp,
          scope: "session",
          phase: "bootstrap",
          occurred_at: occurredAt,
          code: "generic_launch_failure",
          details: diagnostics,
        },
      },
    });
    await apiClient.seedSessionMessage(task.session_id, {
      type: "status",
      content: "Session could not resume",
      createdAt: occurredAt,
      metadata: {
        recovery_actions: true,
        scope: "session",
        error_stamp: stamp,
        error_output: diagnostics,
        actions: [
          {
            type: "ws_request",
            label: "Resume session",
            test_id: "recovery-resume-button",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: task.session_id, action: "resume" },
            },
          },
          {
            type: "ws_request",
            label: "Start fresh session",
            test_id: "recovery-fresh-button",
            params: {
              method: "session.recover",
              payload: { task_id: task.id, session_id: task.session_id, action: "fresh_start" },
            },
          },
        ],
      },
    });
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();
    const row = testPage.getByTestId("session-recovery-card");
    await expect(row).toHaveCount(1);
    await testPage.screenshot({
      path: testInfo.outputPath("recovery-collapsed.png"),
      fullPage: true,
    });
    const primary = row.getByTestId("recovery-resume-button");
    await expect(primary).toBeVisible();
    const primaryBox = await primary.boundingBox();
    expect(primaryBox!.height).toBeGreaterThanOrEqual(
      testInfo.project.name === "mobile-chrome" ? 44 : 27,
    );
    if (testInfo.project.name === "chromium") expect(primaryBox!.height).toBeLessThanOrEqual(29);
    await expect(testPage.getByTestId("recovery-fresh-button")).toBeVisible();
    await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(1);
    await expect(row.getByRole("button", { name: "More options" })).toHaveCount(0);
    await row.getByText("Technical details", { exact: true }).click();
    const text = row.locator("pre");
    await expect(text).toContainText("connection refused");
    await expect(row).not.toContainText("synthetic-private-value");
    const box = await text.boundingBox();
    expect(box!.width).toBeGreaterThan(150);
    await expect(text).toHaveCSS("overflow-y", "visible");
    await assertNoDocumentHorizontalOverflow(testPage, "recovery details");
    if (testInfo.project.name === "chromium") {
      for (const width of [2048, 767, 768, 769, 1440]) {
        await testPage.setViewportSize({ width, height: 1000 });
        if (!(await text.isVisible()))
          await row.getByText("Technical details", { exact: true }).click();
        await expect(text).toBeVisible();
        await expect.poll(async () => (await text.boundingBox())?.width ?? 0).toBeGreaterThan(150);
        await assertNoDocumentHorizontalOverflow(testPage, `recovery at ${width}px`);
      }
    }
    await testPage.evaluate(() => {
      document.documentElement.style.fontSize = "200%";
    });
    await assertNoDocumentHorizontalOverflow(testPage, "recovery at 200% text size");
    await testPage.screenshot({
      path: testInfo.outputPath("recovery-large-text.png"),
      fullPage: true,
    });
    await testPage.evaluate(() => {
      document.documentElement.style.fontSize = "";
    });
    await testPage.evaluate(() => {
      Object.defineProperty(navigator, "clipboard", {
        configurable: true,
        value: {
          writeText: async (value: string) => {
            (window as unknown as { copiedDiagnostic: string }).copiedDiagnostic = value;
          },
        },
      });
    });
    await row.getByRole("button", { name: "Copy details", exact: true }).click();
    const copied = await testPage.evaluate(
      () => (window as unknown as { copiedDiagnostic: string }).copiedDiagnostic,
    );
    expect(copied).toBe(await text.textContent());
    expect(copied).not.toContain("synthetic-private-value");
    await testPage.screenshot({
      path: testInfo.outputPath("recovery-details.png"),
      fullPage: true,
    });
    if (testInfo.project.name === "mobile-chrome") {
      await testPage.setViewportSize({ width: 1024, height: 900 });
      await expect(primary).toBeVisible();
      expect((await primary.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await expect(row.getByTestId("recovery-fresh-button")).toBeVisible();
      await assertNoDocumentHorizontalOverflow(testPage, "coarse-pointer tablet recovery");
      await testPage.screenshot({
        path: testInfo.outputPath("recovery-tablet-options.png"),
        fullPage: true,
      });
    }
  });
}

export function uniformRecoveryCases() {
  resolvedFailureScenario();
  for (const scenario of [
    { name: "failed startup", state: "FAILED", kind: "generic" },
    { name: "interrupted waiting", state: "WAITING_FOR_INPUT", kind: "generic" },
    { name: "runtime installation", state: "FAILED", kind: "managed_runtime_npm_resolution" },
    { name: "provider quota", state: "FAILED", kind: "provider_quota_limited" },
  ] as const) {
    test(`one composer recovery owner: ${scenario.name}`, async ({
      testPage,
      apiClient,
      seedData,
    }, testInfo) => {
      const task = await apiClient.createTask(seedData.workspaceId, scenario.name, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        agent_profile_id: seedData.agentProfileId,
        repository_ids: [seedData.repositoryId],
      });
      const summary = "Session needs recovery";
      const stamp = "uniform-fixture";
      const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
        state: scenario.state,
        ...(scenario.state === "FAILED" ? { completedAt: new Date().toISOString() } : {}),
        agentProfileId: seedData.agentProfileId,
        errorMessage: summary,
        metadata: { last_agent_error: { message: summary, stamp } },
      });
      await apiClient.seedSessionMessage(sessionId, {
        type: "status",
        content: summary,
        metadata: {
          recovery_actions: true,
          error_stamp: stamp,
          failure_kind: scenario.kind,
          provider_name: "OpenCode",
          model_id: "mock-model",
          remediation_url:
            scenario.kind === "provider_quota_limited"
              ? "https://opencode.ai/workspace/demo/go"
              : undefined,
          error_output: "Authorization: Bearer synthetic-private-value\nConnection refused",
        },
      });
      await testPage.goto(`/t/${task.id}`);
      const card = testPage.getByTestId("session-recovery-card");
      await expect(card).toBeVisible();
      await expect(card).toHaveCount(1);
      await expect(testPage.locator('[contenteditable="true"]:visible')).toHaveCount(0);
      await expect(
        testPage
          .getByTestId("session-recovery-history")
          .getByRole("button", { name: /Resume|Retry|Start fresh/ }),
      ).toHaveCount(0);
      if (scenario.kind === "generic") {
        await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(1);
        await expect(card.getByTestId("recovery-fresh-button")).toBeVisible();
      } else if (scenario.kind === "managed_runtime_npm_resolution") {
        await expect(card.getByTestId("managed-runtime-npm-retry-button")).toBeVisible();
        await expect(testPage.getByTestId("recovery-resume-button")).toHaveCount(0);
      } else {
        await expect(card).toContainText("OpenCode");
        await testPage.setViewportSize({ width: 320, height: 900 });
        const providerLink = card.getByTestId("remediation-link");
        await expect(providerLink).toHaveAttribute("href", "https://opencode.ai/workspace/demo/go");
        await providerLink.click({ trial: true });
        await expect(card.getByTestId("recovery-resume-button")).toBeVisible();
        await expect(card.getByTestId("recovery-fresh-button")).toHaveCount(0);
        await expect(card.getByRole("button", { name: /Delete|Archive|Change model/ })).toHaveCount(
          0,
        );
      }
      await card.getByText("Technical details", { exact: true }).click();
      await expect(card.locator("pre")).toContainText("Connection refused");
      await expect(card).not.toContainText("synthetic-private-value");
      await assertNoDocumentHorizontalOverflow(testPage, scenario.name);
      await testPage.screenshot({
        path: testInfo.outputPath("uniform-recovery.png"),
        fullPage: true,
      });
      await testPage.reload();
      await expect(card).toHaveCount(1);
    });
  }
}

function resolvedFailureScenario() {
  test("resolved runtime failure does not own recovery for a still-failed session", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTask(seedData.workspaceId, "Resolved runtime failure", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      repository_ids: [seedData.repositoryId],
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "FAILED",
      completedAt: new Date().toISOString(),
      agentProfileId: seedData.agentProfileId,
      errorMessage: "Connection lost",
      metadata: {
        last_agent_error: {
          message: "Runtime installation failed",
          stamp: "resolved-runtime",
          occurred_at: "2026-09-20T10:00:00Z",
        },
        recovery_resolved_at: "2026-09-20T11:00:00Z",
      },
    });
    await apiClient.seedSessionMessage(sessionId, {
      type: "status",
      content: "Runtime installation failed",
      createdAt: "2026-09-20T10:00:00Z",
      metadata: {
        recovery_actions: true,
        error_stamp: "resolved-runtime",
        failure_kind: "managed_runtime_npm_resolution",
      },
    });
    await testPage.goto(`/t/${task.id}`);
    const banner = testPage.getByTestId("failed-session-banner");
    await expect(banner).toBeVisible();
    await expect(testPage.getByTestId("session-recovery-card")).toHaveCount(0);
    await expect(testPage.getByTestId("managed-runtime-npm-retry-button")).toHaveCount(0);
    await expect(testPage.locator('[contenteditable="true"]:visible')).toHaveCount(0);
    await assertNoDocumentHorizontalOverflow(testPage, "resolved runtime recovery");
  });
}

export function recoveryDraftScenario() {
  test("preserves draft across a blocked composer and recovered session", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Recovery draft continuity",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("fixture session missing");
    await waitForSessionDone(apiClient, task.id, task.session_id, "draft fixture settled");
    await testPage.goto(`/t/${task.id}`);
    const editor = testPage.locator('[contenteditable="true"]:visible').first();
    await editor.fill("Keep this unsent draft");
    await testPage
      .getByTestId("session-chat")
      .locator('input[type="file"]')
      .setInputFiles({
        name: "recovery-draft.txt",
        mimeType: "text/plain",
        buffer: Buffer.from("Preserve this attachment"),
      });
    await expect(testPage.getByText("recovery-draft.txt", { exact: false })).toBeVisible();
    await setDraftSessionError(testPage, task.session_id, "Connection lost");
    const card = testPage.getByTestId("session-recovery-card");
    await expect(card).toBeVisible();
    await expect(editor).toHaveCount(0);
    await setDraftSessionError(testPage, task.session_id, "", "STARTING");
    await expect(card.getByTestId("recovery-resume-button")).toBeDisabled();
    await testPage.screenshot({
      path: testInfo.outputPath("recovery-pending.png"),
      fullPage: true,
    });
    await card.focus();
    await setDraftSessionError(testPage, task.session_id, "");
    await expect(card).toHaveCount(0);
    await expect(editor).toContainText("Keep this unsent draft");
    await expect(editor).toBeFocused();
    await expect(testPage.getByText("recovery-draft.txt", { exact: false })).toBeVisible();
  });
}

// The seed endpoint persists snapshots; this transition explicitly exercises the live UI store.
async function setDraftSessionError(
  page: Page,
  sessionId: string,
  error: string,
  nextState: "WAITING_FOR_INPUT" | "STARTING" = "WAITING_FOR_INPUT",
) {
  await page.evaluate(
    ({ sessionId, error, nextState }) => {
      const store = (window as Window & { __KANDEV_E2E_STORE__?: { getState: () => AppState } })
        .__KANDEV_E2E_STORE__;
      if (!store) throw new Error("E2E store bridge missing");
      const state = store.getState();
      const current = state.taskSessions.items[sessionId];
      if (!current) throw new Error("Session not loaded");
      state.setTaskSession({
        ...current,
        state: nextState,
        error_message: error,
        metadata: error
          ? { last_agent_error: { message: error, stamp: "draft-interruption" } }
          : {},
      });
    },
    { sessionId, error, nextState },
  );
}
