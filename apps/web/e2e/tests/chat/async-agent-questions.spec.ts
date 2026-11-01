import { test, expect } from "../../fixtures/test-base";
import {
  openTaskSession,
  waitForAgentMessage,
  waitForSessionDone,
  waitForSessionState,
} from "../../helpers/session";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";

async function createAsyncQuestionTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
  scenario: string,
) {
  return apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
    description: scenario,
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    repository_ids: [seedData.repositoryId],
  });
}

test.describe("Async agent questions", () => {
  test("asks early, keeps working, and answers from the Inbox", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);
    const title = "Async Keep Working";
    const task = await createAsyncQuestionTask(apiClient, seedData, title, "/e2e:async-question");
    if (!task.session_id) throw new Error("expected an active session for the async question flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "RUNNING",
      message: "the author should still be working while the question is open",
      timeout: 10_000,
    });
    await testPage.goto("/needs-you-inbox");

    const group = testPage.getByTestId("async-question-group").filter({ hasText: title });
    await expect(group).toBeVisible({ timeout: 30_000 });
    const row = group.getByTestId("async-question-row").first();
    await expect(row.getByText("Agent continues work")).toBeVisible({ timeout: 15_000 });

    await row.getByTestId("async-question-row-toggle").click();
    await row.getByTestId("async-question-option").filter({ hasText: "PostgreSQL" }).click();
    await row.getByTestId("async-question-submit").click();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
    await waitForAgentMessage(
      apiClient,
      task.session_id,
      "ASYNC_ANSWER_RECEIVED: PostgreSQL",
      45_000,
    );
  });

  test("parks an explicit dependency wait and resumes after the answer", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const title = "Async Explicit Wait";
    const task = await createAsyncQuestionTask(
      apiClient,
      seedData,
      title,
      "/e2e:async-question-wait",
    );
    if (!task.session_id) throw new Error("expected an active session for the async wait flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WAIT_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the explicit question wait should park the captured turn",
      timeout: 60_000,
    });

    await testPage.goto("/needs-you-inbox");
    const group = testPage.getByTestId("async-question-group").filter({ hasText: title });
    await expect(group).toBeVisible({ timeout: 30_000 });
    const row = group.getByTestId("async-question-row").first();
    await expect(row.getByText("Waiting for answer")).toBeVisible({ timeout: 15_000 });

    await row.getByTestId("async-question-row-toggle").click();
    await row.getByTestId("async-question-option").filter({ hasText: "SQLite" }).click();
    await row.getByTestId("async-question-submit").click();

    await expect(group).toHaveCount(0, { timeout: 20_000 });
    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_ANSWER_RECEIVED: SQLite", 45_000);
  });

  test("answers from the chat strip while the agent keeps working", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await createAsyncQuestionTask(
      apiClient,
      seedData,
      "Async Chat Answer",
      "/e2e:async-question",
    );
    if (!task.session_id) throw new Error("expected an active session for the async chat flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "RUNNING",
      message: "chat answer must not cancel or supersede the running turn",
      timeout: 10_000,
    });

    const session = await openTaskSession(testPage, task.id);
    const strip = testPage.getByTestId("task-async-question-strip");
    await expect(strip).toBeVisible({ timeout: 20_000 });
    await strip.getByTestId("task-async-question-toggle").click();
    const row = strip.getByTestId("async-question-row").first();
    await row.getByTestId("async-question-row-toggle").click();
    await row.getByTestId("async-question-option").filter({ hasText: "PostgreSQL" }).click();
    await row.getByTestId("async-question-submit").click();

    await expect(session.chat).toBeVisible();
    await waitForAgentMessage(
      apiClient,
      task.session_id,
      "ASYNC_ANSWER_RECEIVED: PostgreSQL",
      45_000,
    );
    await waitForSessionDone(
      apiClient,
      task.id,
      task.session_id,
      "the original running turn should complete after reading its answer",
      15_000,
    );
  });

  test("resumes an explicit wait after an answer from task chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await createAsyncQuestionTask(
      apiClient,
      seedData,
      "Async Wait From Chat",
      "/e2e:async-question-wait",
    );
    if (!task.session_id) throw new Error("expected an active session for the async wait flow");

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WAIT_WORK_MARKER", 60_000);
    await waitForSessionState(apiClient, {
      taskId: task.id,
      sessionId: task.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "the explicit question wait should park the captured turn",
      timeout: 60_000,
    });

    await openTaskSession(testPage, task.id);
    const strip = testPage.getByTestId("task-async-question-strip");
    await expect(strip).toBeVisible({ timeout: 20_000 });
    await strip.getByTestId("task-async-question-toggle").click();
    const row = strip.getByTestId("async-question-row").first();
    await row.getByTestId("async-question-row-toggle").click();
    await row.getByTestId("async-question-option").filter({ hasText: "SQLite" }).click();
    await row.getByTestId("async-question-submit").click();

    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_ANSWER_RECEIVED: SQLite", 45_000);
  });

  test("forwards two child questions and answers one from the parent chat", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(180_000);
    const parent = await createAsyncQuestionTask(
      apiClient,
      seedData,
      "Async Forward Parent",
      "/e2e:async-parent-forward-watch",
    );
    if (!parent.session_id) throw new Error("expected an active parent session");

    const createChild = (title: string) =>
      apiClient.createTaskWithAgent(seedData.workspaceId, title, seedData.agentProfileId, {
        description: "/e2e:async-parent-question",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
        parent_id: parent.id,
        workspace_mode: "inherit_parent",
        autopilot: true,
      });
    const [firstChild, secondChild] = await Promise.all([
      createChild("Async Forward Child A"),
      createChild("Async Forward Child B"),
    ]);
    if (!firstChild.session_id || !secondChild.session_id) {
      throw new Error("expected active child sessions for parent forwarding");
    }

    await Promise.all(
      [firstChild, secondChild].map((child) =>
        waitForAgentMessage(apiClient, child.session_id!, "ASYNC_PARENT_WAIT_MARKER", 60_000),
      ),
    );
    await Promise.all(
      [firstChild, secondChild].map((child) =>
        waitForSessionState(apiClient, {
          taskId: child.id,
          sessionId: child.session_id!,
          expectedState: "WAITING_FOR_INPUT",
          message: `${child.title} should wait for its parent answer`,
          timeout: 60_000,
        }),
      ),
    );

    await waitForAgentMessage(apiClient, parent.session_id, "ASYNC_PARENT_FORWARDED:", 60_000);
    await openTaskSession(testPage, parent.id);
    const strip = testPage.getByTestId("task-async-question-strip");
    await expect(strip).toBeVisible({ timeout: 20_000 });
    const toggle = strip.getByTestId("task-async-question-toggle");
    await toggle.click();
    await expect(toggle).toHaveAttribute("aria-expanded", "true");
    await expect(strip.getByTestId("async-question-row")).toHaveCount(2, { timeout: 20_000 });
    const firstRow = strip
      .getByTestId("async-question-row")
      .filter({ hasText: "Async Forward Child A" });
    await expect(firstRow.getByTestId("async-question-provenance")).toBeVisible();
    await firstRow.getByTestId("async-question-row-toggle").click();
    await firstRow.getByTestId("async-question-option").filter({ hasText: "PostgreSQL" }).click();
    await firstRow.getByTestId("async-question-submit").click();

    await waitForAgentMessage(
      apiClient,
      parent.session_id,
      "ASYNC_PARENT_FORWARD_ANSWER_RECEIVED: PostgreSQL",
      45_000,
    );
    await waitForAgentMessage(
      apiClient,
      firstChild.session_id,
      "ASYNC_ANSWER_RECEIVED: PostgreSQL",
      45_000,
    );
    await waitForSessionState(apiClient, {
      taskId: secondChild.id,
      sessionId: secondChild.session_id,
      expectedState: "WAITING_FOR_INPUT",
      message: "answering one child must leave the sibling request pending",
      timeout: 10_000,
    });
  });

  test("withdrawal stays in the author transcript with its reason", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);
    const task = await createAsyncQuestionTask(
      apiClient,
      seedData,
      "Async Withdraw Transcript",
      "/e2e:async-question-withdraw",
    );
    if (!task.session_id) throw new Error("expected an active session for the withdrawal flow");
    await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WITHDRAWN:", 60_000);
    await waitForAgentMessage(apiClient, task.session_id, "ASYNC QUESTION REQUEST", 20_000);

    await expect
      .poll(
        async () => {
          const page = await apiClient.listSessionMessages(task.session_id!);
          return page.messages.some(
            (message) =>
              message.type === "agent_question_request" &&
              typeof message.metadata?.async_question_id === "string",
          );
        },
        { timeout: 20_000, message: "the transcript outbox should publish the request once" },
      )
      .toBe(true);
    await openTaskSession(testPage, task.id);
    const transcriptQuestion = testPage.getByTestId("agent-question-message");
    await expect(transcriptQuestion).toBeVisible({ timeout: 20_000 });
    await expect(transcriptQuestion.getByText("Withdrawn")).toBeVisible();
    await expect(
      transcriptQuestion.getByText("Found the database choice in the project docs."),
    ).toBeVisible();
    await expect(testPage.getByTestId("task-async-question-strip")).toHaveCount(0);
  });

  test("answers from task chat when the Inbox feature is disabled", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const restoreBackend = await backend.useEnv({ KANDEV_FEATURES_NEEDS_YOU_INBOX: "false" });
    try {
      const task = await createAsyncQuestionTask(
        apiClient,
        seedData,
        "Async Inbox Disabled",
        "/e2e:async-question",
      );
      if (!task.session_id) throw new Error("expected an active session for the Inbox-off flow");
      await waitForAgentMessage(apiClient, task.session_id, "ASYNC_WORK_MARKER", 60_000);
      await openTaskSession(testPage, task.id);
      const strip = testPage.getByTestId("task-async-question-strip");
      await expect(strip).toBeVisible({ timeout: 20_000 });
      await strip.getByTestId("task-async-question-toggle").click();
      const row = strip.getByTestId("async-question-row").first();
      await row.getByTestId("async-question-row-toggle").click();
      await row.getByTestId("async-question-option").filter({ hasText: "MongoDB" }).click();
      await row.getByTestId("async-question-submit").click();
      await waitForAgentMessage(
        apiClient,
        task.session_id,
        "ASYNC_ANSWER_RECEIVED: MongoDB",
        45_000,
      );
    } finally {
      await restoreBackend();
    }
  });
});
