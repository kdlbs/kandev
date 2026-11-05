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
        facts: {
          schema_version: 1,
          process: {
            state: "running",
            checked_at: now,
            valid_until: new Date(nowMs + 30_000).toISOString(),
          },
          launch_checks: {
            state: "passed",
            receipt: {
              checked_at: now,
              policy_digest: "c".repeat(64),
              package_digest: "a".repeat(64),
              runtime_digest: "b".repeat(64),
              checks: [
                "policy_ceiling",
                "runtime_checksum",
                "private_artifacts",
                "process_containment",
              ],
            },
          },
          runtime: {
            state: "reported",
            observations: [
              {
                source_id: "runtime.v2",
                evidence_digest: "e".repeat(64),
                observed_at: new Date(nowMs - 60_000).toISOString(),
                platform: "linux",
                landlock_abi: 5,
                execute_enforced: true,
                command_mediation_active: true,
              },
            ],
          },
          audit: { integrity: "unchecked", finalization: "unknown", signer: "unchecked" },
        },
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
    const facts = dialog.getByTestId("execution-facts");
    await expect(facts.getByRole("heading", { name: "Launch checks", exact: true })).toBeAttached();
    await expect(facts.getByText("Passed", { exact: true })).toBeAttached();
    await expect(facts.getByText("Detected Landlock ABI", { exact: true })).toBeAttached();
    await expect(facts.getByTestId("execution-process-state")).toHaveText("Running");
    await assertNoDocumentHorizontalOverflow(testPage, "execution details");
    const body = dialog.getByTestId("execution-details-body");
    expect(await body.evaluate((element) => getComputedStyle(element).overscrollBehaviorY)).toBe(
      "contain",
    );
    await facts.scrollIntoViewIfNeeded();
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
    if (!mobile) {
      await testPage.setViewportSize({ width: 767, height: 851 });
      // Open the selected record from its menu without changing the active session.
      await testPage.getByTestId("mobile-sessions-pill").click();
      await testPage
        .getByTestId(`mobile-session-row-${sessionId}`)
        .getByRole("button", { name: "Session actions" })
        .click();
      await testPage.getByRole("menuitem", { name: "Execution details", exact: true }).click();
      const back = dialog.getByRole("button", { name: "Back", exact: true });
      await expect(back).toBeVisible();
      expect((await back.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(testPage, "narrow fine-pointer execution facts");
      await testPage.setViewportSize({ width: 768, height: 851 });
      await expect(testPage.getByTestId("mobile-sessions-pill")).not.toBeVisible();
      await expect(trigger).toBeVisible();
      if (!(await dialog.isVisible())) await trigger.click();
      await expect(dialog.getByRole("button", { name: "Close", exact: true })).toBeVisible();
      expect((await dialog.boundingBox())!.width).toBeLessThan(768);
    }
    await testPage.evaluate((id) => {
      const state = (
        window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
      ).__KANDEV_E2E_STORE__.getState();
      const session = state.taskSessions.items[id];
      const observation = session.execution_observation!;
      const checked = new Date(Date.now() - 27_000).toISOString();
      state.upsertTaskSessionFromEvent(session.task_id, {
        ...session,
        execution_observation: {
          ...observation,
          revision: observation.revision + 1,
          checked_at: new Date().toISOString(),
          status: "unknown",
          provider_health: "unavailable",
          facts: {
            ...observation.facts!,
            process: {
              state: "running",
              checked_at: checked,
              valid_until: new Date(Date.parse(checked) + 30_000).toISOString(),
            },
          },
        },
      });
    }, sessionId);
    await expect(facts.getByTestId("execution-process-state")).toHaveText("Running");
    await expect(dialog.getByRole("status").filter({ hasText: /^Unknown$/ })).toBeVisible();
    await expect(facts.getByTestId("execution-process-state")).toHaveText("Unknown", {
      timeout: 6000,
    });
    await expect(facts.getByText("Passed", { exact: true })).toBeAttached();
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(trigger).toHaveAccessibleName(/Unknown/);
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
