import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { watchWs, dwell } from "../../helpers/causal-waits";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { formatBytes } from "../../../lib/utils/format-bytes";

// ---------------------------------------------------------------------------
// Overview
// ---------------------------------------------------------------------------
//
// Covers REQ-TASKS-PLAN-CONTENT-SIZE-LIMIT-003 (browser-visible rejection):
// a plan write submitted over the 262,144-byte ceiling is rejected end to end
// (real browser -> gateway WS -> PlanService -> WS error frame -> banner),
// the user's draft is kept, the panel does not resubmit it, and shrinking the
// draft back under the ceiling saves normally and clears the banner.
//
// This is a real E2E test, not a component test, because the assertions live
// on both sides of a network boundary the component tests (use-task-plan.test.ts,
// task-plan-panel-draft.test.ts) cannot cross: the actual bytes the backend
// received and rejected, and the actual bytes still sitting in the browser's
// editor DOM afterward.

const CEILING_BYTES = 262_144; // planinjection ceiling; kept in sync with plan_size.go's MaxPlanContentBytes
const MARKER = "PLAN-SIZE-CEILING-MARKER";
const SUBMITTED_BYTES = 300_000;

/** ASCII filler so content.length (UTF-16 code units) equals UTF-8 byte length exactly. */
function contentOfByteLength(totalBytes: number): string {
  const filler = "A".repeat(totalBytes - MARKER.length);
  return `${MARKER}${filler}`;
}

/** Dispatch a synthetic plain-text paste into the ProseMirror plan editor.
 * Avoids `keyboard.type`, which would take minutes for a 300,000-character
 * string; a real user pasting a large document is exactly the scenario this
 * capability defends against. */
async function pasteIntoPlanEditor(session: SessionPage, text: string) {
  const editor = session.planEditor();
  await editor.click();
  await editor.evaluate((element, pasted) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", pasted);
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
    element.dispatchEvent(pasteEvent);
  }, text);
}

async function openPlanPanel(session: SessionPage) {
  if (await session.planPanel.isVisible()) return;
  await session.togglePlanMode();
  await expect(session.planPanel).toBeVisible({ timeout: 10_000 });
}

/**
 * Ground truth for what the backend actually rejected: parsed out of the WS
 * error frame rather than assumed from the input string length, since the
 * editor's markdown round-trip may not preserve byte-for-byte length. The
 * banner is required to echo these exact numbers (AC-003.1), not re-derive
 * them from the draft, so the test must know the real ones to check that.
 */
async function expectPlanWriteRejectedForSize(
  pending: Promise<unknown>,
): Promise<{ limit: number; submitted: number }> {
  try {
    await pending;
    throw new Error("expected the plan write to be rejected for content size");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    expect(message).toContain("plan_content_too_large");
    const match = message.match(/"limit":(\d+).*"submitted":(\d+)/);
    if (!match) throw new Error(`could not parse limit/submitted out of rejection: ${message}`);
    return { limit: Number(match[1]), submitted: Number(match[2]) };
  }
}

/** JSON-string-escape content for embedding in a mock-agent script line. */
function mcpWrite(content: string): string {
  return JSON.stringify(content).slice(1, -1);
}

test.describe("Plan panel — content size ceiling", () => {
  test.describe.configure({ retries: 1 });

  test("oversized draft is rejected, banner names the ceiling and size, draft is kept, no resubmission", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // Attached before the first navigation so the capture sees the gateway
    // socket from the moment it opens (page.on("websocket") only fires for
    // sockets opened after the listener attaches).
    const traffic = attachGatewayTrafficCapture(testPage);
    const wsWatcher = watchWs(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plan size ceiling — reject",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await openPlanPanel(session);

    // No plan exists yet, so the first save is a create.
    const rejectedCreate = wsWatcher.waitForResponse("task.plan.create", { timeout: 15_000 });

    const oversized = contentOfByteLength(SUBMITTED_BYTES);
    await pasteIntoPlanEditor(session, oversized);

    const rejection = await expectPlanWriteRejectedForSize(rejectedCreate);
    expect(rejection.limit).toBe(CEILING_BYTES);
    expect(rejection.submitted).toBeGreaterThan(CEILING_BYTES);

    // Banner names both numbers, taken from the rejection's `details` — not
    // re-derived from the editor (AC-003.1).
    const banner = session.planPanel.getByTestId("plan-save-error-banner");
    await expect(banner).toBeVisible({ timeout: 10_000 });
    await expect(banner).toContainText(formatBytes(rejection.limit));
    await expect(banner).toContainText(formatBytes(rejection.submitted));

    // AC-003.2: the draft is not cleared or reverted.
    await expect(session.planEditor()).toContainText(MARKER);

    // AC-003.1/001.4: nothing was stored — no plan exists for this task.
    await expect(await apiClient.getTaskPlan(task.id)).toBeNull();

    // AC-003.4: the panel does not resubmit the rejected draft. Wait past
    // several autosave debounce windows (1500ms) and confirm no further
    // task.plan.create request went out beyond the one already observed.
    // This is a permanent negative assertion (nothing should ever happen
    // here while the draft is unchanged), so it uses the sanctioned wall-clock
    // dwell rather than a budgeted causal wait.
    await dwell(
      testPage,
      4_000,
      "negative-assertion",
      "confirms autosave does not resubmit content rejected for size",
    );
    const createRequests = traffic.frames.filter(
      (frame) => frame.direction === "sent" && frame.action === "task.plan.create",
    );
    expect(createRequests).toHaveLength(1);
  });

  test("shrinking the draft below the ceiling saves normally and clears the banner", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const wsWatcher = watchWs(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plan size ceiling — recover",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });
    await openPlanPanel(session);

    const rejectedCreate = wsWatcher.waitForResponse("task.plan.create", { timeout: 15_000 });
    await pasteIntoPlanEditor(session, contentOfByteLength(SUBMITTED_BYTES));
    await expectPlanWriteRejectedForSize(rejectedCreate);

    const banner = session.planPanel.getByTestId("plan-save-error-banner");
    await expect(banner).toBeVisible({ timeout: 10_000 });

    // Replace the draft with something well under the ceiling: select all,
    // delete, and type the short replacement (small enough that real keyboard
    // input is fast, and it exercises the normal typing path rather than a
    // second synthetic paste).
    const shortContent = "Shortened plan after size rejection.";
    const acceptedCreate = wsWatcher.waitForResponse("task.plan.create", { timeout: 15_000 });
    const editor = session.planEditor();
    await editor.click();
    const modifier = process.platform === "darwin" ? "Meta" : "Control";
    await testPage.keyboard.press(`${modifier}+a`);
    await testPage.keyboard.press("Backspace");
    await testPage.keyboard.type(shortContent);

    // AC-003.5: at/below the ceiling and otherwise valid, the write is admitted.
    await expect(acceptedCreate).resolves.toBeTruthy();

    // Banner clears once the successful attempt begins (AC-003.5) and stays
    // clear once it completes.
    await expect(banner).toBeHidden({ timeout: 10_000 });
    await expect
      .poll(async () => (await apiClient.getTaskPlan(task.id))?.content, { timeout: 10_000 })
      .toBe(shortContent);
  });

  test("existing plan: oversized edit is rejected via update, banner shows, stored plan unchanged", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    // This exercises the update branch (savePlan calls updateTaskPlan once a
    // plan already exists), distinct from the create branch the other two
    // tests exercise — a task's first save and every later save are different
    // client code paths, both required to enforce the identical ceiling
    // (AC-001.3), and this is the only test here that starts from a stored plan.
    const initialPlan = "Initial small plan.";
    const seedScript = [
      `e2e:mcp:kandev:create_task_plan_kandev({"task_id":"{task_id}","content":"${mcpWrite(initialPlan)}"})`,
      'e2e:message("Plan seeded.")',
    ].join("\n");

    // Armed before the first navigation: the gateway socket opens once at
    // page load, and a watcher attached afterward would never see it (see
    // watchWs's doc comment).
    const wsWatcher = watchWs(testPage);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Plan size ceiling — update reject",
      seedData.agentProfileId,
      {
        description: seedScript,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect.poll(() => apiClient.getTaskPlan(task.id), { timeout: 30_000 }).not.toBeNull();
    await session.waitForChatIdle({ timeout: 45_000 });
    await openPlanPanel(session);
    await expect(session.planPanel).toContainText(initialPlan, { timeout: 15_000 });

    // AC-003.7: opening the panel on a task whose stored plan already exists
    // (and is well under the ceiling) must not show a false rejection.
    const banner = session.planPanel.getByTestId("plan-save-error-banner");
    await expect(banner).toHaveCount(0);

    const rejectedUpdate = wsWatcher.waitForResponse("task.plan.update", { timeout: 15_000 });
    await pasteIntoPlanEditor(session, contentOfByteLength(SUBMITTED_BYTES));
    const rejection = await expectPlanWriteRejectedForSize(rejectedUpdate);
    expect(rejection.limit).toBe(CEILING_BYTES);
    expect(rejection.submitted).toBeGreaterThan(CEILING_BYTES);

    await expect(banner).toBeVisible({ timeout: 10_000 });
    await expect(banner).toContainText(formatBytes(rejection.limit));
    await expect(banner).toContainText(formatBytes(rejection.submitted));

    // The stored plan is untouched by the rejected update.
    await expect
      .poll(async () => (await apiClient.getTaskPlan(task.id))?.content, { timeout: 10_000 })
      .toBe(initialPlan);
  });
});
