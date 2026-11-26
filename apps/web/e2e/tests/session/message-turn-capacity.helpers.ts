import { expect, type Locator, type Page } from "@playwright/test";
import type { BackendContext } from "../../fixtures/backend";
import type { SeedData } from "../../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient, QueueSessionIdentityInput } from "../../helpers/api-client";
import { assertVerticalCentersAligned } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";
import { waitForSessionState } from "../workflow/queued-session-ownership-helpers";

export type MessageCapacityFixture = {
  target: CreateTaskResponse & { session_id: string };
  filler: CreateTaskResponse & { session_id: string };
  session: SessionPage;
  identity: QueueSessionIdentityInput;
  releaseCeiling: () => Promise<void>;
};

const CAPACITY_NOTICE = "Message queued because the session limit was reached.";
export const CAPACITY_AUTO_RUN = "It will send automatically when capacity is available.";
export const CAPACITY_AUTO_RUN_OFF = "Turn on Auto-run to send it automatically.";

export function capacityNotice(session: SessionPage): Locator {
  return session.activeChat().getByTestId("chat-input-area").getByTestId("queue-capacity-notice");
}

export async function expectCollapsedCapacityNotice(
  session: SessionPage,
  autoRunCopy: typeof CAPACITY_AUTO_RUN | typeof CAPACITY_AUTO_RUN_OFF,
): Promise<void> {
  const chat = session.activeChat();
  await expect(chat.getByTestId("queue-chip")).toBeVisible();
  await expect(chat.getByTestId("queued-ghost-list")).toBeHidden();
  const notice = capacityNotice(session);
  await expect(notice).toHaveCount(1);
  await expect(notice).toBeVisible();
  await expect(notice).toContainText(CAPACITY_NOTICE);
  await expect(notice).toContainText("The current session limit is 1.");
  await expect(notice).toContainText(autoRunCopy);
  await expect(notice).not.toContainText(
    autoRunCopy === CAPACITY_AUTO_RUN ? CAPACITY_AUTO_RUN_OFF : CAPACITY_AUTO_RUN,
  );
  await expect(chat.getByRole("status").filter({ hasText: CAPACITY_NOTICE })).toHaveCount(1);

  const hasYellowBackground = await notice.evaluate((element) => {
    const canvas = document.createElement("canvas");
    canvas.width = canvas.height = 1;
    const context = canvas.getContext("2d")!;
    context.fillStyle = getComputedStyle(element).backgroundColor;
    context.fillRect(0, 0, 1, 1);
    const [red = 0, green = 0, blue = 0, alpha = 0] = context.getImageData(0, 0, 1, 1).data;
    return alpha > 0 && red >= green && green > blue;
  });
  expect(hasYellowBackground, "capacity notice has a yellow background").toBe(true);
  await assertVerticalCentersAligned(
    notice.locator("svg"),
    notice.getByText(CAPACITY_NOTICE),
    "capacity notice clock and text",
  );
}

const TARGET_DESCRIPTION = "/e2e:simple-message";

export async function expectOneAgentReply(
  apiClient: ApiClient,
  fixture: MessageCapacityFixture,
  marker: string,
): Promise<void> {
  const reply = fixture.session
    .activeChat()
    .getByTestId("agent-message-highlight")
    .filter({ hasText: marker });
  await expect(reply).toHaveCount(1, { timeout: 60_000 });
  await expect(reply).toBeVisible();

  const sessionId = fixture.target.session_id;
  await expect
    .poll(
      async () => {
        const { messages } = await apiClient.listSessionMessages(sessionId);
        return messages.filter(
          (message) => message.author_type === "agent" && message.content.includes(marker),
        ).length;
      },
      {
        timeout: 60_000,
        message: `session ${sessionId} must persist one response for ${marker}`,
      },
    )
    .toBe(1);
}

/**
 * Prepare one warm idle target conversation and a separate live filler. The
 * instance ceiling is enabled first so every subsequent new session competes
 * for the same single slot.
 */
export async function seedMessageCapacityFixture(
  page: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  backend: BackendContext,
  title: string,
): Promise<MessageCapacityFixture> {
  const releaseCeiling = await backend.useEnv({ KANDEV_MAX_CONCURRENT_SESSIONS: "1" });
  let filler: (CreateTaskResponse & { session_id: string }) | undefined;
  try {
    const target = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `${title} target`,
      seedData.agentProfileId,
      {
        description: TARGET_DESCRIPTION,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!target.session_id) throw new Error("message capacity target has no session_id");
    await waitForSessionState(apiClient, target.id, target.session_id, "WAITING_FOR_INPUT", 60_000);

    const capacityMarker = `${title} filler accepted`;
    filler = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      `${title} filler`,
      seedData.agentProfileId,
      {
        description: `/e2e:cancel-hold ${capacityMarker}`,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!filler.session_id) throw new Error("message capacity filler has no session_id");
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(filler!.session_id!);
          return messages.some(
            (message) =>
              message.author_type === "agent" && message.content.trim() === capacityMarker,
          );
        },
        { timeout: 30_000, message: "capacity filler must acknowledge its live prompt" },
      )
      .toBe(true);
    await waitForSessionState(apiClient, filler.id, filler.session_id, "RUNNING");

    await page.goto(`/t/${target.id}`);
    const session = new SessionPage(page);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });
    const identity = await apiClient.getQueueSessionIdentity(target.id, target.session_id);
    return { target, filler, session, identity, releaseCeiling };
  } catch (error) {
    if (filler?.session_id) {
      await apiClient
        .stopSession({
          session_id: filler.session_id,
          reason: "message capacity setup cleanup",
          force: true,
        })
        .catch(() => undefined);
    }
    await releaseCeiling();
    throw error;
  }
}

export async function cleanupMessageCapacityFixture(
  apiClient: ApiClient,
  fixture: MessageCapacityFixture,
): Promise<void> {
  await apiClient
    .stopSession({
      session_id: fixture.filler.session_id,
      reason: "message capacity e2e cleanup",
      force: true,
    })
    .catch(() => undefined);
  await apiClient.deleteTask(fixture.filler.id, { cascade: true }).catch(() => undefined);
  await apiClient.deleteTask(fixture.target.id, { cascade: true }).catch(() => undefined);
  await fixture.releaseCeiling();
}

export async function submitFollowUp(
  fixture: MessageCapacityFixture,
  marker: string,
  isMobile: boolean,
): Promise<void> {
  const chat = fixture.session.activeChat();
  const editor = chat.getByTestId("chat-input-editor");
  const submit = fixture.session.submitButton();
  await expect(editor).toHaveAttribute("contenteditable", "true");
  await editor.fill(`e2e:message("${marker}")`);
  await expect(submit).toBeEnabled();
  if (isMobile) await submit.tap();
  else await submit.click();
  await expect(editor).toHaveText("");
}
