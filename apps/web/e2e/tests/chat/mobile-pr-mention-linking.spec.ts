import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  PR_NUMBER,
  PR_URL,
  entityReferencesFromMessage,
  prepareQueuedPullRequestContext,
  preparePullRequestMentionTask,
  pullRequestReference,
  reselectQueuedPullRequestForSave,
  routePullRequestSearch,
  waitForFiniteAnimations,
} from "./pr-mention-linking-fixtures";

test.describe("Mobile PR mention linking", () => {
  test.describe.configure({ timeout: 90_000 });

  test("uses the registration drawer and links the accepted message", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await preparePullRequestMentionTask(
      apiClient,
      seedData,
      "Mobile direct PR mention registration",
    );
    if (!task.session_id) throw new Error("PR mention task did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await routePullRequestSearch(testPage);

    const editor = await session.composerReady();
    await editor.tap();
    await editor.fill("");
    await editor.pressSequentially("#E2E selected pull request");
    const option = testPage.getByRole("option").filter({ hasText: "E2E selected pull request" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.tap();

    const draftChip = editor.getByTestId("entity-reference-chip");
    const modeTrigger = draftChip.getByTestId("reference-registration-trigger");
    await expect(modeTrigger).toContainText("Link on send");
    await modeTrigger.tap();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    const contextChoice = drawer.getByTestId("reference-registration-context_only");
    await expect(contextChoice).toHaveCSS("min-height", "44px");
    await contextChoice.tap();
    await expect(modeTrigger).toContainText("Context only");
    await drawer.getByRole("button", { name: "Done" }).tap();
    await expect(drawer).not.toBeVisible();

    await modeTrigger.tap();
    const linkChoice = testPage
      .getByRole("dialog")
      .getByTestId("reference-registration-link_on_send");
    await linkChoice.tap();
    await testPage.getByRole("dialog").getByRole("button", { name: "Done" }).tap();
    await expect(modeTrigger).toContainText("Link on send");

    await editor.pressSequentially(" Please review this change.");
    await session.activeChat().getByTestId("submit-message-button").tap();

    const reference = pullRequestReference();
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          const accepted = messages.find((message) =>
            entityReferencesFromMessage(message).some((item) => item.id === reference.id),
          );
          return accepted
            ? entityReferencesFromMessage(accepted)[0]?.registration_status
            : undefined;
        },
        { timeout: 30_000, message: "Wait for mobile selected PR registration to complete" },
      )
      .toBe("linked");

    await expect
      .poll(async () => (await apiClient.listTaskPRs(task.id)).map((item) => item.pr_number), {
        timeout: 30_000,
        message: "Wait for mobile selected PR in the persisted task list",
      })
      .toEqual([PR_NUMBER]);

    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const accepted = messages.find((message) =>
      entityReferencesFromMessage(message).some((item) => item.id === reference.id),
    );
    expect(accepted).toBeDefined();
    expect(entityReferencesFromMessage(accepted!)[0]).toMatchObject({
      association_mode: "link_on_send",
      registration_status: "linked",
      selection_id: expect.any(String),
      url: PR_URL,
    });
  });

  test("reselects a queued PR with the save drawer and links only after Save", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await preparePullRequestMentionTask(
      apiClient,
      seedData,
      "Mobile queued PR relink",
    );
    if (!task.session_id) throw new Error("PR mention task did not return a session_id");
    const { identity, content } = await prepareQueuedPullRequestContext(
      apiClient,
      task.id,
      task.session_id,
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await routePullRequestSearch(testPage);
    const queueChip = testPage.getByTestId("queue-chip");
    await expect(queueChip).toBeVisible({ timeout: 10_000 });
    await queueChip.tap();
    const row = testPage.getByTestId("queue-entry");
    await expect(row).toBeVisible();

    await reselectQueuedPullRequestForSave(testPage, row);
    await expect(testPage.getByTestId("queue-edit-textarea")).toHaveValue(content);
    const modeTrigger = testPage.getByTestId("reference-registration-trigger");
    await modeTrigger.tap();
    const drawer = testPage.getByRole("dialog");
    await expect(drawer).toBeVisible();
    const saveChoice = drawer.getByTestId("reference-registration-link_on_send");
    await expect(saveChoice).toHaveCSS("min-height", "44px");
    await expect(saveChoice).toHaveAttribute("aria-checked", "true");
    if (prCapture.capturing) {
      await waitForFiniteAnimations(testPage);
      await prCapture.screenshot("queue-reference-save-drawer", {
        caption: "A queued pull request is selected to link when the edit is saved.",
      });
    }
    await drawer.getByRole("button", { name: "Done" }).tap();
    await testPage.getByRole("button", { name: "Save", exact: true }).tap();

    await expect
      .poll(
        async () => {
          const status = await apiClient.getQueueStatus(identity);
          const references = status.entries[0]?.metadata?.entity_references;
          return Array.isArray(references)
            ? (references[0] as { registration_status?: string }).registration_status
            : undefined;
        },
        { timeout: 30_000, message: "Wait for mobile queue edit registration to finish" },
      )
      .toBe("linked");
    await expect
      .poll(async () => (await apiClient.listTaskPRs(task.id)).map((pr) => pr.pr_number), {
        timeout: 30_000,
        message: "Wait for the queue reference in the persisted task PR list",
      })
      .toEqual([PR_NUMBER]);
    const acceptedQueue = await apiClient.getQueueStatus(identity);
    expect(acceptedQueue.entries[0]?.content).toBe(content);
    expect(acceptedQueue.entries[0]?.metadata?.entity_references).toMatchObject([
      {
        association_mode: "link_on_send",
        registration_status: "linked",
        selection_id: expect.not.stringMatching(/^selection-queued-context$/u),
      },
    ]);
  });
});
