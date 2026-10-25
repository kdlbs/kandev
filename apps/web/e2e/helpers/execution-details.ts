import { randomUUID } from "node:crypto";
import { expect, test } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export async function projectExecutionFixture(page: Page, id: string) {
  await page.evaluate((sessionId) => {
    const store = (window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } })
      .__KANDEV_E2E_STORE__;
    const state = store.getState();
    const session = state.taskSessions.items[sessionId];
    const nowMs = Date.now();
    const now = new Date(nowMs).toISOString();
    state.upsertTaskSessionFromEvent(session.task_id, {
      ...session,
      name: "Captured execution session",
      execution_snapshot: {
        schema_version: 1,
        attempt_id: "ui-attempt",
        session_id: sessionId,
        task_id: session.task_id,
        execution_id: "ui-execution",
        generation: 1,
        sequence: 100,
        executor_id: "ui-executor",
        executor_type: "local",
        agent_profile_id: session.agent_profile_id ?? "",
        transport: "acp",
        captured_at: now,
        agent: { executable: "fixture-agent", omitted_arguments: 2, references: [] },
        wrapper: {
          source: "profile",
          digest: "a".repeat(64),
          command: { executable: "fixture-wrapper", omitted_arguments: 4, references: [] },
        },
        sandbox: {
          attempt_id: "ui-attempt",
          generation: 1,
          installation_id: "ui-install",
          provider_key: "fixture-provider",
          provider_version: "1.0",
          package_digest: "a".repeat(64),
          runtime_version: "2.0",
          runtime_digest: "b".repeat(64),
          revision_id: "ui-revision",
          revision_number: 7,
          policy_name: "Captured policy " + "long-name-".repeat(20),
          policy_digest: "c".repeat(64),
          selection_source: "task",
          ceiling_revision_id: "ui-ceiling",
          ceiling_digest: "d".repeat(64),
        },
      },
      execution_observation: {
        schema_version: 1,
        observer_generation: 100,
        revision: 1,
        session_id: sessionId,
        task_id: session.task_id,
        snapshot_sequence: 100,
        attempt_id: "ui-attempt",
        generation: 1,
        attempt_version: 3,
        status: "active",
        provider_health: "available",
        reason: "",
        mechanism: "fixture-kernel",
        checked_at: now,
        evidence_at: now,
        valid_until: new Date(nowMs + 30_000).toISOString(),
      },
    });
  }, id);
}

export function executionDetailsScenario() {
  test("keeps execution history and status scoped to one session", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Execution details", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const { session_id: siblingId } = await apiClient.seedTaskSession(task.id, {
      sessionId: randomUUID(),
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    expect(sessionId).toBeTruthy();
    expect(siblingId).toBeTruthy();
    expect(siblingId).not.toBe(sessionId);
    await testPage.goto(`/t/${task.id}`);
    if (mobile) {
      await testPage.getByTestId("mobile-sessions-pill").click();
      await testPage.getByTestId(`mobile-session-row-${sessionId}`).click();
    }
    const trigger = testPage.getByTestId(`execution-control-${sessionId}`);
    await expect(trigger).toBeVisible();
    await projectExecutionFixture(testPage, sessionId);
    await expect(trigger).toHaveAccessibleName(/Active/);
    const activeBefore = await testPage.evaluate(
      () =>
        (
          window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
        ).__KANDEV_E2E_STORE__.getState().tasks.activeSessionId,
    );
    if (mobile) {
      expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await trigger.tap();
    } else {
      await trigger.focus();
      await testPage.keyboard.press("Enter");
    }
    const dialog = testPage.getByRole("dialog", { name: "Execution details", exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("fixture-wrapper", { exact: true })).toBeVisible();
    await expect(dialog.getByText("Verified mechanism: fixture-kernel")).toBeVisible();
    await expect(dialog.getByText(/Denial reports are unavailable/)).toBeAttached();
    await assertNoDocumentHorizontalOverflow(testPage, "execution details");
    const body = dialog.getByTestId("execution-details-body");
    expect(await body.evaluate((element) => getComputedStyle(element).overscrollBehaviorY)).toBe(
      "contain",
    );
    await dialog.screenshot({ path: testInfo.outputPath("execution-details.png") });
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await expect.poll(async () => Math.round((await dialog.boundingBox())!.width)).toBe(width);
        expect(
          (await dialog.getByRole("button", { name: "Back", exact: true }).boundingBox())!.height,
        ).toBeGreaterThanOrEqual(44);
        await assertNoDocumentHorizontalOverflow(testPage, `execution details at ${width}`);
      }
    }
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toBeFocused();
    expect(
      await testPage.evaluate(
        () =>
          (
            window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
          ).__KANDEV_E2E_STORE__.getState().tasks.activeSessionId,
      ),
    ).toBe(activeBefore);
    if (mobile) {
      await testPage.getByTestId("mobile-sessions-pill").click();
      await testPage
        .getByTestId(`mobile-session-row-${siblingId}`)
        .getByRole("button", { name: "Session actions" })
        .click();
      await testPage.getByRole("menuitem", { name: "Execution details", exact: true }).click();
    } else {
      await testPage.getByTestId(`session-tab-${siblingId}`).click({ button: "right" });
      await testPage.getByRole("menuitem", { name: "Execution details", exact: true }).click();
    }
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText("No launch record is available for this session.")).toBeVisible();
    await expect(dialog.getByText("fixture-wrapper", { exact: true })).not.toBeAttached();
  });
}
