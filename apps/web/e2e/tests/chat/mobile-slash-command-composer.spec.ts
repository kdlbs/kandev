import { type Page } from "@playwright/test";
import { test, expect } from "../../fixtures/test-base";
import { seedAvailableCommands, seedConfirmedConfigOptions } from "../../helpers/session-store";
import { attachMessageAddCapture } from "../../helpers/ws-capture";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../../lib/types/http";

const SLOW_COMMAND = {
  name: "slow",
  description: "Run a slow response",
  input_hint: "duration",
};

const RETRO_SKILL = {
  name: "$retro",
  kind: "skill",
  description:
    "Use this skill to run a retrospective, gather useful evidence, and record focused improvements for future work.",
};

const LEGACY_RETRO_COMMAND = {
  name: "retro",
  description: "Legacy command with the same visible name",
};

const PLAN_COMMAND = {
  name: "plan",
  description: "Set plan mode",
  action: {
    kind: "set_config_option",
    config_id: "collaboration_mode",
    value: "plan",
    reset_value: "default",
  },
};

const GOAL_COMMAND = {
  name: "goal",
  description: "Set a goal to keep pursuing",
  input_hint: "<objective>|clear|pause|resume",
};

async function createReadyTask(
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<CreateTaskResponse> {
  return apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Mobile Slash Command Draft",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
}

async function openTaskChat(page: Page, taskId: string): Promise<SessionPage> {
  await page.goto(`/t/${taskId}`);
  const session = new SessionPage(page);
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  return session;
}

test.describe("Mobile slash command composer", () => {
  test("touches a clean skill row, keeps a draft, and sends the raw provider command", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const messages = attachMessageAddCapture(testPage);
    const task = await createReadyTask(apiClient, seedData);
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);
    const editor = await session.composerReady();
    await seedAvailableCommands(testPage, task.session_id, [
      RETRO_SKILL,
      LEGACY_RETRO_COMMAND,
      PLAN_COMMAND,
      GOAL_COMMAND,
    ]);
    await seedConfirmedConfigOptions(testPage, task.session_id, { collaboration_mode: "plan" });
    const originalMessageCount = messages.frames.filter((frame) => frame.taskId === task.id).length;

    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("/");

    const plan = testPage.getByRole("option", {
      name: /\/plan.*Mode.*Active.*Turn plan mode off/,
    });
    const goal = testPage.getByRole("option", {
      name: /\/goal.*Set a goal to keep pursuing.*Arguments: <objective>/,
    });
    await expect(plan).toBeVisible();
    await expect(goal).toBeVisible();
    await expect(goal).not.toContainText("Skill");
    await expect(goal).not.toContainText("Mode");

    const planBounds = await plan.boundingBox();
    const labelBounds = await plan.getByText("/plan", { exact: true }).boundingBox();
    const modeBounds = await plan.getByText("Mode", { exact: true }).boundingBox();
    const activeBounds = await plan.getByText("Active", { exact: true }).boundingBox();
    const descriptionBounds = await plan
      .getByText("Turn plan mode off", { exact: true })
      .boundingBox();
    expect(planBounds).not.toBeNull();
    expect(labelBounds).not.toBeNull();
    expect(modeBounds).not.toBeNull();
    expect(activeBounds).not.toBeNull();
    expect(descriptionBounds).not.toBeNull();
    expect(planBounds!.height).toBeGreaterThanOrEqual(44);
    expect(planBounds!.x).toBeGreaterThanOrEqual(0);
    expect(planBounds!.y).toBeGreaterThanOrEqual(0);
    expect(planBounds!.x + planBounds!.width).toBeLessThanOrEqual(
      await testPage.evaluate(() => window.innerWidth),
    );
    expect(labelBounds!.x + labelBounds!.width).toBeLessThanOrEqual(modeBounds!.x);
    expect(modeBounds!.x + modeBounds!.width).toBeLessThanOrEqual(activeBounds!.x);
    expect(activeBounds!.x + activeBounds!.width).toBeLessThanOrEqual(descriptionBounds!.x);
    const listbox = testPage.getByRole("listbox", { name: "Commands" });
    const listBounds = await listbox.boundingBox();
    expect(listBounds).not.toBeNull();
    expect(listBounds!.x).toBeGreaterThanOrEqual(0);
    expect(listBounds!.y).toBeGreaterThanOrEqual(0);
    expect(listBounds!.x + listBounds!.width).toBeLessThanOrEqual(
      await testPage.evaluate(() => window.innerWidth),
    );
    expect(listBounds!.y + listBounds!.height).toBeLessThanOrEqual(
      await testPage.evaluate(() => window.innerHeight),
    );
    await testPage.screenshot({ path: test.info().outputPath("mobile-skill-command-menu.png") });

    await editor.fill("");
    await editor.pressSequentially("/retro");
    const skill = testPage.getByRole("option", { name: /\/retro.*Skill/ });
    const legacy = testPage.getByRole("option", { name: /\/retro.*Legacy command/ });
    await expect(skill).toBeVisible();
    await expect(legacy).toBeVisible();
    await expect(testPage.getByRole("option")).toHaveCount(2);
    const skillBounds = await skill.boundingBox();
    const skillLabelBounds = await skill.getByText("/retro", { exact: true }).boundingBox();
    const badgeBounds = await skill.getByText("Skill", { exact: true }).boundingBox();
    const longDescriptionBounds = await skill
      .getByText(RETRO_SKILL.description, { exact: true })
      .boundingBox();
    expect(skillBounds).not.toBeNull();
    expect(skillLabelBounds).not.toBeNull();
    expect(badgeBounds).not.toBeNull();
    expect(longDescriptionBounds).not.toBeNull();
    expect(skillBounds!.height).toBeGreaterThanOrEqual(44);
    expect(skillBounds!.x).toBeGreaterThanOrEqual(0);
    expect(skillBounds!.x + skillBounds!.width).toBeLessThanOrEqual(
      await testPage.evaluate(() => window.innerWidth),
    );
    expect(skillLabelBounds!.x + skillLabelBounds!.width).toBeLessThanOrEqual(badgeBounds!.x);
    expect(badgeBounds!.x + badgeBounds!.width).toBeLessThanOrEqual(longDescriptionBounds!.x);

    await skill.tap();
    const chip = editor.getByTestId("slash-command-chip");
    await expect(chip).toHaveText("retro");
    await editor.pressSequentially("with context");
    await expect
      .poll(() => messages.frames.filter((frame) => frame.taskId === task.id).length)
      .toBe(originalMessageCount);

    await expect(session.submitButton()).toBeEnabled();
    await session.submitButton().tap();
    await expect
      .poll(() =>
        messages.frames.some(
          (frame) =>
            frame.taskId === task.id &&
            frame.sessionId === task.session_id &&
            frame.content === "/$retro with context",
        ),
      )
      .toBe(true);
  });

  test("tapping a slash command keeps it as a draft", async ({ testPage, apiClient, seedData }) => {
    const task = await createReadyTask(apiClient, seedData);
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    const session = await openTaskChat(testPage, task.id);

    // Resolve the active editor through the same startup-aware gate used by
    // send helpers; the visible idle placeholder can briefly precede TipTap's
    // editable host while the mobile session finishes starting.
    const editor = await session.composerReady();
    // composerReady may reload to reconcile stale startup state. Seed the
    // client-side command list only after that recovery so it is not discarded.
    await seedAvailableCommands(testPage, task.session_id, [SLOW_COMMAND]);
    // @covers AC-UI-COMPOSER-FOCUS-HINT-001.1
    await editor.fill("");
    await editor.blur();
    const composer = session.chat.filter({ visible: true });
    await expect(composer.getByText("to focus", { exact: true })).not.toBeVisible();
    await expect(composer.locator(".pr-28")).toHaveCount(0);
    await testPage.screenshot({ path: test.info().outputPath("mobile-composer.png") });
    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("/s");

    const command = testPage.getByText("/slow", { exact: true });
    await expect(command).toBeVisible({ timeout: 5_000 });
    await command.tap();

    await expect(editor).toHaveText(/slow/, { timeout: 5_000 });
    await expect(editor.getByTestId("slash-command-chip")).toHaveText("slow", { timeout: 5_000 });
    const chatList = session.chat.locator(".chat-message-list:visible");
    await expect(chatList.getByText("/slow", { exact: false })).not.toBeVisible({ timeout: 1_000 });

    await editor.pressSequentially("1s");
    await expect(editor).toHaveText(/slow\s+1s/, { timeout: 5_000 });
    // The submit button shows a spinner and is `disabled` while the auto-started
    // session finishes its brief STARTING transition. Tapping it then is a no-op
    // that drops the message, so wait for it to be enabled before tapping.
    await expect(session.submitButton()).toBeEnabled();
    await session.submitButton().tap();
    await expect(chatList.getByText("/slow 1s", { exact: false })).toBeVisible({
      timeout: 10_000,
    });
  });
});
