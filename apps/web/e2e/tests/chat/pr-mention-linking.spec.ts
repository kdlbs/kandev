import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import {
  PR_NUMBER,
  PR_URL,
  entityReferencesFromMessage,
  preparePullRequestMentionProvider,
  preparePullRequestMentionTask,
  prepareQueuedPullRequestContext,
  pullRequestReference,
  reselectQueuedPullRequestForSave,
  routePullRequestSearch,
  waitForFiniteAnimations,
} from "./pr-mention-linking-fixtures";

test.describe("PR mention linking", () => {
  test.describe.configure({ timeout: 90_000 });

  test("registers the selected pull request after the direct message is accepted", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await preparePullRequestMentionTask(
      apiClient,
      seedData,
      "Direct PR mention registration",
    );
    if (!task.session_id) throw new Error("PR mention task did not return a session_id");
    const repository = await apiClient.getRepository(seedData.repositoryId);
    expect(repository).toMatchObject({
      provider: "github",
      provider_repo_id: "100407",
      provider_host: "https://github.com",
      provider_owner: "testorg",
      provider_name: "e2e-pr-registration",
    });
    const taskSnapshot = await apiClient.getTask(task.id);
    expect(taskSnapshot.repositories?.map((item) => item.repository_id)).toContain(
      seedData.repositoryId,
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });
    await routePullRequestSearch(testPage);

    const editor = await session.composerReady();
    await editor.fill("");
    await editor.pressSequentially("#E2E selected pull request");
    const option = testPage.getByRole("option").filter({ hasText: "E2E selected pull request" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();

    const draftChip = editor.getByTestId("entity-reference-chip");
    await expect(draftChip.getByTestId("reference-registration-trigger")).toContainText(
      "Link on send",
    );
    expect(await apiClient.listTaskPRs(task.id)).toHaveLength(0);
    await editor.pressSequentially(" Please review this change.");
    await session.activeChat().getByTestId("submit-message-button").click();

    const reference = pullRequestReference();
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          const accepted = messages.find((message) =>
            entityReferencesFromMessage(message).some((item) => item.id === reference.id),
          );
          const selected = accepted
            ? entityReferencesFromMessage(accepted).find((item) => item.id === reference.id)
            : undefined;
          return selected
            ? `${selected.registration_status ?? "missing"}:${selected.registration_error_code ?? "none"}:actor=${accepted?.author_id ?? "empty"}`
            : "missing";
        },
        { timeout: 30_000, message: "Wait for selected PR registration to complete" },
      )
      .toMatch(/^linked:none:actor=.+$/);

    await expect
      .poll(
        async () => {
          const taskPRs = await apiClient.listTaskPRs(task.id);
          return taskPRs.map((item) => item.pr_number);
        },
        { timeout: 30_000, message: "Wait for the selected PR in the persisted task list" },
      )
      .toEqual([PR_NUMBER]);

    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const accepted = messages.find((message) =>
      entityReferencesFromMessage(message).some((item) => item.id === reference.id),
    );
    expect(accepted).toBeDefined();
    const storedReference = entityReferencesFromMessage(accepted!)[0];
    expect(storedReference).toMatchObject({
      association_mode: "link_on_send",
      registration_status: "linked",
      selection_id: expect.any(String),
      url: PR_URL,
    });
    expect(storedReference.selection_id).not.toBe("");

    const linkedPR = (await apiClient.listTaskPRs(task.id))[0];
    expect(linkedPR).toMatchObject({
      repository_id: seedData.repositoryId,
      pr_number: PR_NUMBER,
      pr_url: PR_URL,
    });
    const sentChip = session
      .activeChat()
      .locator(".chat-message-list:visible")
      .getByTestId("entity-reference-chip")
      .filter({ hasText: `${PR_NUMBER}` });
    await expect(sentChip).toBeVisible({ timeout: 10_000 });
    await expect(sentChip).toHaveAttribute("href", PR_URL);
  });

  test("registers a selected pull request on the accepted initial task brief", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await preparePullRequestMentionProvider(apiClient, seedData);
    const task = await apiClient.createTask(seedData.workspaceId, "Initial brief PR registration", {
      description: "Preserve this initial brief when accepting the first message.",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
      prepare_session: true,
      repository_ids: [seedData.repositoryId],
    });
    if (!task.session_id) throw new Error("prepared PR mention task did not return a session_id");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 90_000 });
    await routePullRequestSearch(testPage);

    const editor = await session.composerReady();
    await editor.fill("");
    await editor.pressSequentially("#E2E selected pull request");
    const option = testPage.getByRole("option").filter({ hasText: "E2E selected pull request" });
    await expect(option).toBeVisible({ timeout: 10_000 });
    await option.click();
    const draftChip = editor.getByTestId("entity-reference-chip");
    await expect(draftChip.getByTestId("reference-registration-trigger")).toContainText(
      "Link on send",
    );
    await editor.pressSequentially(" Start with the selected pull request.");
    await session.activeChat().getByTestId("submit-message-button").click();

    const reference = pullRequestReference();
    await expect
      .poll(
        async () => {
          const { messages } = await apiClient.listSessionMessages(task.session_id!);
          const accepted = messages.find((message) =>
            entityReferencesFromMessage(message).some((item) => item.id === reference.id),
          );
          const selected = accepted
            ? entityReferencesFromMessage(accepted).find((item) => item.id === reference.id)
            : undefined;
          return selected?.registration_status ?? "missing";
        },
        { timeout: 30_000, message: "Wait for initial-brief PR registration to complete" },
      )
      .toBe("linked");

    const { messages } = await apiClient.listSessionMessages(task.session_id);
    const accepted = messages.find((message) =>
      entityReferencesFromMessage(message).some((item) => item.id === reference.id),
    );
    expect(accepted?.content).toContain("Preserve this initial brief");
    expect(accepted?.content).toContain("Start with the selected pull request");
    expect(await apiClient.listTaskPRs(task.id)).toMatchObject([
      { repository_id: seedData.repositoryId, pr_number: PR_NUMBER, pr_url: PR_URL },
    ]);
  });

  test("saves a queued reference reselection without changing its text", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }) => {
    const task = await preparePullRequestMentionTask(apiClient, seedData, "Queued PR relink");
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
    await queueChip.click();
    const row = testPage.getByTestId("queue-entry");
    await expect(row).toBeVisible();

    await reselectQueuedPullRequestForSave(testPage, row);
    await expect(testPage.getByTestId("reference-registration-trigger")).toBeVisible();
    const selectedReference = testPage.getByTestId("queue-reference-chip");
    const selectedSelectionId = await selectedReference.getAttribute("data-selection-id");
    expect(selectedSelectionId).toBeTruthy();
    expect(selectedSelectionId).not.toBe("selection-queued-context");
    if (prCapture.capturing) {
      await waitForFiniteAnimations(testPage);
      await prCapture.screenshot("queue-editor-link-on-save", {
        caption: "A queued pull request is ready to link when the edit is saved.",
      });
    }
    await testPage.setViewportSize({ width: 390, height: 844 });
    await testPage.getByTestId("queue-chip").click();
    await expect(testPage.getByTestId("queue-edit-textarea")).toHaveValue(content);
    await expect(testPage.getByTestId("queue-reference-chip")).toHaveAttribute(
      "data-selection-id",
      selectedSelectionId!,
    );
    await testPage.setViewportSize({ width: 1280, height: 800 });
    const desktopQueueChip = testPage.getByTestId("queue-chip");
    if (await desktopQueueChip.isVisible()) await desktopQueueChip.click();
    await expect(testPage.getByTestId("queue-edit-textarea")).toHaveValue(content);
    await expect(testPage.getByTestId("queue-reference-chip")).toHaveAttribute(
      "data-selection-id",
      selectedSelectionId!,
    );
    await testPage.reload();

    const reloadedSession = new SessionPage(testPage);
    await reloadedSession.waitForLoad();
    await reloadedSession.waitForChatIdle({ timeout: 30_000 });
    await routePullRequestSearch(testPage);
    await expect(testPage.getByTestId("queue-chip")).toBeVisible({ timeout: 10_000 });
    await testPage.getByTestId("queue-chip").click();
    await expect(testPage.getByTestId("queue-edit-textarea")).toHaveValue(content);
    await expect(testPage.getByTestId("reference-registration-trigger")).toContainText(
      "Link on save",
    );
    await expect(testPage.getByTestId("queue-reference-chip")).toHaveAttribute(
      "data-selection-id",
      selectedSelectionId!,
    );
    await expect.poll(async () => (await apiClient.listTaskPRs(task.id)).length).toBe(0);
    const beforeSave = await apiClient.getQueueStatus(identity);
    expect(beforeSave.entries[0]?.metadata?.entity_references).toMatchObject([
      { association_mode: "context_only", selection_id: "selection-queued-context" },
    ]);

    await testPage.getByRole("button", { name: "Save", exact: true }).click();
    await expect
      .poll(
        async () => {
          const status = await apiClient.getQueueStatus(identity);
          const references = status.entries[0]?.metadata?.entity_references;
          return Array.isArray(references)
            ? (references[0] as { registration_status?: string }).registration_status
            : undefined;
        },
        { timeout: 30_000, message: "Wait for queue edit registration to finish" },
      )
      .toBe("linked");
    await expect
      .poll(async () => (await apiClient.listTaskPRs(task.id)).map((pr) => pr.pr_number))
      .toEqual([PR_NUMBER]);
    const acceptedQueue = await apiClient.getQueueStatus(identity);
    expect(acceptedQueue.entries[0]?.content).toBe(content);
    expect(acceptedQueue.entries[0]?.metadata?.entity_references).toMatchObject([
      {
        association_mode: "link_on_send",
        registration_status: "linked",
        selection_id: selectedSelectionId,
      },
    ]);
  });
});
