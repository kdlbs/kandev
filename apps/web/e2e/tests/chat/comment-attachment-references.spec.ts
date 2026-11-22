import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { attachCommentFile } from "../../helpers/comment-attachment";
import { readMockACPTrace } from "../../helpers/transient-turn-runtime-continuity";
import {
  openSeededAgentReply,
  selectAgentReplyText,
  SELECTED_REPLY_TEXT,
} from "./agent-message-comments-helpers";

// @covers AC-TASKS-COMMENT-FILES-001.3
test("agent receives readable file evidence beside selected output", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  test.setTimeout(120_000);
  const agent = (await apiClient.listAgents()).agents.find((row) => row.name === "mock-agent");
  if (!agent) throw new Error("mock-agent is unavailable");
  const tracePath = path.join(backend.tmpDir, "comment-files.jsonl");
  const profile = await apiClient.createAgentProfile(agent.id, "Comment attachment proof", {
    model: "mock-fast",
    env_vars: [{ key: "E2E_MOCK_AGENT_ACP_TRACE_FILE", value: tracePath }],
  });
  try {
    const { body, session } = await openSeededAgentReply(
      testPage,
      apiClient,
      {
        ...seedData,
        agentProfileId: profile.id,
      },
      "Comment reference delivery",
    );
    const examples = [
      {
        selection: SELECTED_REPLY_TEXT,
        feedback: "Compare the first example.",
        bytes: "first file",
      },
      { selection: "useful detail", feedback: "Compare the second example.", bytes: "second file" },
    ];
    for (const example of examples) {
      await selectAgentReplyText(body, example.selection);
      await testPage.getByTestId("agent-message-comment-trigger").click();
      const form = testPage.getByTestId("agent-message-comment-popover");
      await form.getByRole("textbox").fill(example.feedback);
      await attachCommentFile(testPage, form, "output-evidence.txt", example.bytes);
      await form.getByTestId("agent-message-comment-add").click();
      await expect(form).not.toBeVisible();
    }
    const instruction = "Read both comment files.";
    await session.sendMessageViaButton(instruction);
    await expect
      .poll(
        () => {
          const prompt = readMockACPTrace(tracePath).find(
            (row) => row.event === "prompt" && row.prompt?.includes(instruction),
          )?.prompt;
          return examples.map(({ feedback }) => {
            const reference = prompt
              ?.split(feedback)[1]
              ?.match(/^\s*- "output-evidence.txt": ("[^"\n]+")/);
            return reference && fs.readFileSync(JSON.parse(reference[1]), "utf8");
          });
        },
        { timeout: 30_000 },
      )
      .toEqual(examples.map((example) => example.bytes));
  } finally {
    await apiClient.deleteAgentProfile(profile.id, true);
    fs.rmSync(tracePath, { force: true });
  }
});
