import { expect, test } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

test.describe("durable agent text segments", () => {
  test("keeps ID-less text on both sides of a tool in separate rows after reconnect and reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(120_000);

    const script = [
      'e2e:message("Segment before one. ")',
      'e2e:message("Segment before two.")',
      "e2e:delay(3000)",
      'e2e:tool_use("Segment boundary tool", {"command":"segment-boundary"})',
      'e2e:tool_result("Segment boundary complete")',
      'e2e:message("Segment after one. ")',
      'e2e:message("Segment after two.")',
    ].join("\n");

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Durable text message segments",
      seedData.agentProfileId,
      {
        description: script,
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await expect(
      session.chat.getByText("Segment before one. Segment before two.", { exact: true }),
    ).toBeVisible({
      timeout: 30_000,
    });

    const listed = await apiClient.listTaskSessions(task.id);
    const activeSession = listed.sessions.find((candidate) => candidate.is_primary);
    if (!activeSession) throw new Error("created task has no primary session");

    // This test-only control rejects legacy sessions and proves durable-v1 is active.
    const disconnected = await apiClient.disconnectSessionAgentUpdatesStream(activeSession.id);
    expect(disconnected.disconnected).toBe(true);

    const expected = [
      { type: "message", content: "Segment before one. Segment before two." },
      { type: "tool_call", content: "tool" },
      { type: "message", content: "Segment after one. Segment after two." },
    ];
    const readOutputRows = async () => {
      const { messages } = await apiClient.listSessionMessages(activeSession.id);
      return messages.filter(
        (message) =>
          message.author_type === "agent" &&
          (message.type === "message" || message.type === "tool_call"),
      );
    };
    const assertTranscript = async () => {
      await expect
        .poll(
          async () => {
            const rows = await readOutputRows();
            return rows.map((message) => ({
              type: message.type,
              content: message.type === "tool_call" ? "tool" : message.content,
            }));
          },
          { timeout: 60_000 },
        )
        .toEqual(expected);

      const rows = await readOutputRows();
      const rowOrder = await testPage.evaluate(
        (ids) => {
          const elements = ids.map((id) => document.getElementById(`msg-${id}`));
          if (elements.some((element) => element === null)) return null;
          const [before, tool, after] = elements as HTMLElement[];
          return (
            Boolean(before.compareDocumentPosition(tool) & Node.DOCUMENT_POSITION_FOLLOWING) &&
            Boolean(tool.compareDocumentPosition(after) & Node.DOCUMENT_POSITION_FOLLOWING)
          );
        },
        rows.map((message) => message.id),
      );
      expect(rowOrder).toBe(true);
      await expect(testPage.locator(`#msg-${rows[0].id}`)).toBeVisible();
      await expect(testPage.locator(`#msg-${rows[1].id}`)).toBeVisible();
      await expect(testPage.locator(`#msg-${rows[2].id}`)).toBeVisible();
    };

    await expect(
      session.chat.getByText("Segment after one. Segment after two.", { exact: true }),
    ).toBeVisible({
      timeout: 60_000,
    });
    await assertTranscript();

    await testPage.reload();
    await session.waitForLoad();
    await assertTranscript();
  });
});
