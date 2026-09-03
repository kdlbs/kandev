// Filename starts with "mobile-" so this runs under the mobile-chrome project.
import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import { dwell, watchWs } from "../../helpers/causal-waits";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { formatBytes } from "../../../lib/utils/format-bytes";

const CEILING_BYTES = 262_144;
const SUBMITTED_BYTES = 300_000;
const MARKER = "MOBILE-PLAN-SIZE-CEILING-MARKER";

function contentOfByteLength(totalBytes: number): string {
  return `${MARKER}${"A".repeat(totalBytes - MARKER.length)}`;
}

async function pasteIntoPlanEditor(session: SessionPage, content: string) {
  const editor = session.planEditor();
  // Use a browser-level click before dispatching the paste. The click is
  // supported by touch emulation and the ProseMirror paste handler then sees
  // the same event as a user paste without typing 300,000 characters.
  await editor.click();
  await editor.evaluate((element, pasted) => {
    const clipboardData = new DataTransfer();
    clipboardData.setData("text/plain", pasted);
    const pasteEvent = new Event("paste", { bubbles: true, cancelable: true });
    Object.defineProperty(pasteEvent, "clipboardData", { value: clipboardData });
    element.dispatchEvent(pasteEvent);
  }, content);
}

async function expectSizeRejection(pending: Promise<unknown>) {
  try {
    await pending;
    throw new Error("expected the mobile plan write to be rejected for content size");
  } catch (err) {
    const message = err instanceof Error ? err.message : String(err);
    expect(message).toContain("plan_content_too_large");
    const match = message.match(/"limit":(\d+).*"submitted":(\d+)/);
    if (!match) throw new Error(`could not parse size details from rejection: ${message}`);
    return { limit: Number(match[1]), submitted: Number(match[2]) };
  }
}

test.describe("mobile: plan content size ceiling", () => {
  test("touch plan entry keeps a rejected draft and recovers after shortening", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const wsWatcher = watchWs(testPage);
    const traffic = attachGatewayTrafficCapture(testPage);
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Mobile plan size ceiling",
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

    await session.togglePlanMode();
    await testPage.getByRole("button", { name: "Plan", exact: true }).tap();
    await expect(session.planPanel).toBeVisible({ timeout: 10_000 });

    const rejectedCreate = wsWatcher.waitForResponse("task.plan.create", { timeout: 30_000 });
    await pasteIntoPlanEditor(session, contentOfByteLength(SUBMITTED_BYTES));
    const rejection = await expectSizeRejection(rejectedCreate);
    expect(rejection.limit).toBe(CEILING_BYTES);
    expect(rejection.submitted).toBeGreaterThan(CEILING_BYTES);

    const banner = session.planPanel.getByTestId("plan-save-error-banner");
    await expect(banner).toBeVisible({ timeout: 10_000 });
    await expect(banner).toContainText(formatBytes(rejection.limit));
    await expect(banner).toContainText(formatBytes(rejection.submitted));
    await expect(session.planEditor()).toContainText(MARKER);
    await expect(await apiClient.getTaskPlan(task.id)).toBeNull();

    await dwell(
      testPage,
      4_000,
      "negative-assertion",
      "confirms mobile autosave does not resubmit a size-rejected draft",
    );
    expect(
      traffic.frames.filter(
        (frame) => frame.direction === "sent" && frame.action === "task.plan.create",
      ),
    ).toHaveLength(1);

    const acceptedCreate = wsWatcher.waitForResponse("task.plan.create", { timeout: 15_000 });
    const shortContent = "Shortened mobile plan.";
    await session.planEditor().click();
    await session.planEditor().fill(shortContent);
    await expect(acceptedCreate).resolves.toBeTruthy();
    await expect(banner).toBeHidden({ timeout: 10_000 });
    await expect
      .poll(async () => (await apiClient.getTaskPlan(task.id))?.content, { timeout: 10_000 })
      .toBe(shortContent);
  });
});
