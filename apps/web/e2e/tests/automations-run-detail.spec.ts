import type { Page } from "@playwright/test";
import { test, expect } from "../fixtures/test-base";
import type { ApiClient } from "../helpers/api-client";
import { AutomationsPage } from "../pages/automations-page";

/**
 * The automation run view on a desktop viewport.
 *
 * Three things moved here and none of them can be checked against an empty
 * page: the standing instruction left the top of the transcript for a control
 * in the runs rail, the composer took the run page's own background, and the
 * model picker comes down once the run parks. Every test seeds a real run with
 * a real conversation — a page with no run renders no transcript, no composer
 * and no toolbar, which would pass most of what follows while broken.
 */

type Seed = {
  workspaceId: string;
  workflowId: string;
  startStepId: string;
  agentProfileId: string;
  repositoryId: string;
};

const STANDING_INSTRUCTION = "Check the overnight drift report and summarise what changed.";

/** An automation with one finished run that has a conversation behind it. */
async function seedRunWithConversation(
  apiClient: ApiClient,
  seed: Seed,
  name: string,
  prompt = STANDING_INSTRUCTION,
) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seed.workspaceId,
    name,
    workflowId: seed.workflowId,
    workflowStepId: seed.startStepId,
    prompt,
  });
  const task = await apiClient.createTask(seed.workspaceId, `${name} — run`, {
    workflow_id: seed.workflowId,
    workflow_step_id: seed.startStepId,
  });
  // Parked, which is the state an automation run rests in: the agent has gone
  // but the run stays repliable.
  await apiClient.seedAutomationTaskSession(task.id, "WAITING_FOR_INPUT");
  await apiClient.seedAutomationRun(automation.id, "succeeded", task.id);
  return { automation, task };
}

async function openRunView(testPage: Page, automationId: string) {
  await testPage.goto(`/automations/${automationId}`);
  await expect(testPage.getByTestId("runs-rail")).toBeVisible({ timeout: 15_000 });
  await expect(testPage.getByTestId("run-transcript")).toBeVisible({ timeout: 15_000 });
}

async function seedOpenRun(apiClient: ApiClient, seed: Seed, name: string) {
  const automation = await apiClient.seedAutomation({
    workspaceId: seed.workspaceId,
    name,
    workflowId: seed.workflowId,
    workflowStepId: seed.startStepId,
    prompt: STANDING_INSTRUCTION,
    agentProfileId: seed.agentProfileId,
  });
  const task = await apiClient.createTaskWithAgent(
    seed.workspaceId,
    `${name} — running task`,
    seed.agentProfileId,
    {
      description: "/sleep 30",
      workflow_id: seed.workflowId,
      workflow_step_id: seed.startStepId,
      repository_ids: [seed.repositoryId],
    },
  );
  await apiClient.setTaskOrigin(task.id, "automation_run");
  await expect
    .poll(
      async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        return sessions.find((session) => session.id === task.session_id)?.state;
      },
      { timeout: 30_000 },
    )
    .toBe("RUNNING");
  const run = await apiClient.seedAutomationRun(automation.id, "task_created", task.id);
  expect(run.session_id).not.toBe("");
  expect(run.turn_id).not.toBe("");
  return { automation, task };
}

test.describe("Automation run detail disclosure", () => {
  test("keeps the standing instruction out of the transcript and behind a rail control", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedRunWithConversation(apiClient, seedData, "Rail Disclosure");
    await openRunView(testPage, automation.id);

    // The instruction is the same long text on every run, so nothing of it sits
    // above the conversation the reader came for.
    await expect(testPage.getByTestId("automation-prompt")).toHaveCount(0);
    await expect(testPage.getByTestId("run-detail-panel")).toHaveCount(0);

    // It has moved rather than been deleted: the control lives in the rail.
    const disclosure = testPage.getByTestId("runs-rail").getByTestId("run-detail-toggle");
    await expect(disclosure).toBeVisible();
    await expect(disclosure).toHaveAttribute("aria-expanded", "false");

    await disclosure.click();
    await expect(disclosure).toHaveAttribute("aria-expanded", "true");
    const panel = testPage.getByTestId("run-detail-panel");
    await expect(panel).toBeVisible();
    // The seeded instruction itself, not merely a card-shaped container.
    await expect(panel.getByTestId("automation-prompt")).toContainText(STANDING_INSTRUCTION);
    // The next-firing line rides along, because the topbar drops it on a phone.
    await expect(panel.getByTestId("run-detail-next-run")).not.toHaveText("");

    // Shuts again — it is a disclosure, not a one-way reveal.
    await disclosure.click();
    await expect(testPage.getByTestId("run-detail-panel")).toHaveCount(0);
  });
});

test.describe("Automation exact-run controls", () => {
  test("stops the selected running run and leaves it failed", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedOpenRun(apiClient, seedData, "Exact Stop");
    await openRunView(testPage, automation.id);

    const stop = testPage.getByRole("button", { name: "Stop current run" });
    await expect(stop).toBeVisible({ timeout: 15_000 });
    await stop.click();

    await expect(testPage.getByTestId("run-group-completed")).toBeVisible({ timeout: 15_000 });
    await expect(testPage.getByTestId("run-group-completed")).toContainText("Failed");
    await expect(testPage.getByRole("button", { name: "Stop current run" })).toHaveCount(0);
  });
});

test.describe("Automation run composer", () => {
  test("keeps a reply visible in the complete shared transcript", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Shared transcript reply",
      seedData.agentProfileId,
      {
        description: 'e2e:message("initial shared turn")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((session) => session.id === task.session_id)?.state;
        },
        { timeout: 60_000 },
      )
      .toBe("WAITING_FOR_INPUT");

    await apiClient.setTaskOrigin(task.id, "automation_run");
    const turns = await apiClient.listSessionTurns(task.session_id!);
    expect(turns.turns).toHaveLength(1);
    const initialTurnId = turns.turns[0].id;
    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Shared transcript automation",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt: STANDING_INSTRUCTION,
      agentProfileId: seedData.agentProfileId,
    });
    const run = await apiClient.seedAutomationRun(automation.id, "succeeded", task.id, {
      sessionId: task.session_id,
      turnId: initialTurnId,
    });
    expect(run.session_id).not.toBe("");
    expect(run.turn_id).not.toBe("");

    await openRunView(testPage, automation.id);
    const transcript = testPage.getByTestId("run-transcript");
    await expect(transcript).toContainText("initial shared turn", { timeout: 15_000 });

    const reply = "Reply stays visible in the selected run";
    const editor = transcript.getByTestId("chat-input-editor");
    await expect(editor).toBeEditable({ timeout: 15_000 });
    await editor.fill(reply);
    const submit = transcript.getByTestId("submit-message-button");
    await expect(submit).toBeEnabled({ timeout: 15_000 });
    await submit.click();

    await expect
      .poll(async () => (await apiClient.listSessionTurns(task.session_id!)).turns.length, {
        timeout: 15_000,
      })
      .toBeGreaterThanOrEqual(2);
    const updatedTurns = await apiClient.listSessionTurns(task.session_id!);
    const replyTurnId = updatedTurns.turns.find((turn) => turn.id !== initialTurnId)?.id;
    expect(replyTurnId).toBeTruthy();
    await expect(transcript).toContainText(reply, { timeout: 15_000 });
    // A turn can also contain a synthetic empty-turn status row. Scope the
    // assertion to the row that contains the submitted reply instead of
    // requiring the turn id to identify a single DOM node.
    await expect(
      transcript.locator(`[data-turn-id="${replyTurnId}"]`).filter({ hasText: reply }),
    ).toBeVisible();
  });

  test("sits on the run page's own background, not the task workbench's card", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { automation } = await seedRunWithConversation(apiClient, seedData, "Composer Surface");
    await openRunView(testPage, automation.id);

    const composer = testPage.getByTestId("chat-input-area");
    await expect(composer).toBeVisible({ timeout: 15_000 });

    // `bg-card` is the lighter plate that lifts the composer off the task
    // workbench. On the run page the whole surface is `bg-background` from the
    // topbar down, and a lighter strip along the bottom read as a leftover
    // panel. Asserting the resolved colour rather than the class name means a
    // rename of the utility cannot quietly pass this.
    const [composerBackground, pageBackground] = await Promise.all([
      composer.evaluate((el) => getComputedStyle(el).backgroundColor),
      testPage.evaluate(() => getComputedStyle(document.body).backgroundColor),
    ]);
    expect(composerBackground).toBe(pageBackground);
    // …and the card class is genuinely gone, not merely overpainted.
    await expect(composer).not.toHaveClass(/(^|\s)bg-card(\s|$)/);
  });

  test("drops the model picker once the run parks but keeps the reply box", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // A real agent session, not a seeded row: the model picker only renders
    // once a session has reported its model config, so a synthetic session
    // would make its absence prove nothing.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Parked Run Model Picker",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    // Wait for the turn to end and the session to park — the state the run view
    // is read in, and the state with no agent process behind it.
    await expect
      .poll(
        async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          return sessions.find((session) => session.id === task.session_id)?.state;
        },
        { timeout: 60_000 },
      )
      .toBe("WAITING_FOR_INPUT");

    // The same session is reachable from the task page, where the picker is
    // expected — the control that proves the session really does have a model
    // to pick, so its absence on the run page is the deliberate hiding and not
    // an unconfigured session.
    await testPage.goto(`/t/${task.id}`);
    await expect(testPage.getByTestId("toolbar-item-model")).toBeVisible({ timeout: 30_000 });

    const automation = await apiClient.seedAutomation({
      workspaceId: seedData.workspaceId,
      name: "Parked Run",
      workflowId: seedData.workflowId,
      workflowStepId: seedData.startStepId,
      prompt: STANDING_INSTRUCTION,
    });
    await apiClient.seedAutomationRun(automation.id, "succeeded", task.id);

    await openRunView(testPage, automation.id);

    // The run stays repliable — the composer is the reason this surface exists.
    await expect(testPage.getByTestId("chat-input-area")).toBeVisible({ timeout: 15_000 });
    // The picker is not merely disabled: a control that talks to a process that
    // no longer exists must not be on screen looking operable.
    await expect(testPage.getByTestId("toolbar-item-model")).toHaveCount(0);
  });
});

/**
 * The amber "Paused" note in the topbar.
 *
 * A single-slot automation with one run open is the ordinary steady state of
 * every run it will ever do, so saying Paused there made the normal case look
 * broken. The cap is only news once it is genuinely queueing something.
 *
 * These automations are created through the settings UI rather than seeded:
 * the seeding endpoint has no schedule trigger, and without one the note reads
 * "No schedule" — which would satisfy a "does not say Paused" assertion for
 * entirely the wrong reason.
 */
test.describe("Automation concurrency note", () => {
  /**
   * Create a scheduled automation through the real editor and return its id.
   * The editor's default concurrency cap is one slot, which is the shape this
   * note is about.
   */
  async function createScheduledAutomation(
    testPage: Page,
    seed: Seed & { steps: { name: string }[] },
    name: string,
    maxConcurrentRuns = 1,
  ): Promise<string> {
    const automations = new AutomationsPage(testPage, seed.workspaceId);
    await automations.gotoNew();
    await automations.nameInput.fill(name);
    // Select the schedule explicitly. A new automation starts unscheduled, so
    // changing the time alone cannot create a trigger. 03:17 keeps the real
    // cron scheduler's one-minute-a-day firing window well away from the test.
    await automations.selectFrequency("every day");
    await automations.timeInput.fill("03:17");
    await automations.selectWorkflow("E2E Workflow");
    if (maxConcurrentRuns !== 1) {
      await testPage.getByRole("spinbutton").fill(String(maxConcurrentRuns));
    }
    await expect(automations.saveButton).toBeEnabled({ timeout: 5_000 });
    await automations.saveButton.click();
    await expect(testPage).toHaveURL(/automations$/, { timeout: 15_000 });
    await expect(automations.table).toBeVisible({ timeout: 10_000 });
    await automations.openByName(name);
    await expect(testPage).toHaveURL(/automations\/[a-f0-9-]+$/, { timeout: 10_000 });
    return new URL(testPage.url()).pathname.split("/").pop()!;
  }

  test("stays quiet at one open run and speaks up when the cap really bites", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automationId = await createScheduledAutomation(testPage, seedData, "Steady State");

    const openRun = async (title: string) => {
      const task = await apiClient.createTask(seedData.workspaceId, title, {
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      });
      await apiClient.seedAutomationRun(automationId, "task_created", task.id);
    };

    await openRun("Steady State run 1");
    await testPage.goto(`/automations/${automationId}`);
    const note = testPage.getByTestId("automation-next-run");
    await expect(note).toBeVisible({ timeout: 15_000 });
    // One slot, one run: this is what every run of this automation looks like
    // while it is happening, so the note reports the schedule, not a fault.
    await expect(note).not.toContainText("Paused");
    // …and it reports it because there IS one. Without this the assertion above
    // would also pass on "No schedule", which is what a lost trigger looks like.
    await expect(note).not.toContainText("No schedule");

    // Two runs against a one-slot cap is a genuinely queued firing, and the
    // note names the cap so the reader knows which setting to change. Without
    // this half, the assertion above would pass on a note that never says
    // Paused at all.
    await openRun("Steady State run 2");
    await testPage.reload();
    await expect(note).toContainText("Paused", { timeout: 15_000 });
    await expect(note).toContainText("max 1 at a time");
  });

  test("picks up a manually triggered run without anyone pressing Refresh", async ({
    testPage,
    seedData,
  }) => {
    const automationId = await createScheduledAutomation(testPage, seedData, "Trigger Settle");

    await testPage.goto(`/automations/${automationId}`);
    await expect(testPage.getByTestId("runs-rail-empty")).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId("automation-run-now").click();
    await expect(
      testPage.getByRole("region", { name: /^Notifications/ }).getByText(/Triggered|Skipped/),
    ).toBeVisible({ timeout: 15_000 });

    // The run row is written after the fire returns, so a page that only polls
    // while it can already see something open never learns the run happened.
    // No reload here on purpose — the page has to catch up on its own.
    await expect(testPage.getByTestId("runs-rail-empty")).toHaveCount(0, { timeout: 30_000 });
    // …and nothing it renders from the pre-trigger snapshot is left standing.
    await expect(testPage.getByTestId("automation-next-run")).not.toContainText("Paused");
  });

  test("targets the trigger notification when the run rail has the same status", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const automationId = await createScheduledAutomation(
      testPage,
      seedData,
      "Duplicate Triggered",
      2,
    );
    await apiClient.seedAutomationRun(automationId, "triggered");

    await testPage.goto(`/automations/${automationId}`);
    const runsRail = testPage.getByTestId("runs-rail");
    await expect(runsRail.getByText("Triggered", { exact: true })).toBeVisible({ timeout: 15_000 });

    await testPage.getByTestId("automation-run-now").click();
    await expect(
      testPage
        .getByRole("region", { name: /^Notifications/ })
        .getByText("Triggered", { exact: true }),
    ).toBeVisible({ timeout: 15_000 });
  });
});
