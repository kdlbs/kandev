import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
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
      const trigger = form.getByTestId("comment-attachment-trigger");
      const manager = testPage.getByTestId("comment-attachment-manager");
      const add = form.getByTestId("agent-message-comment-add");
      await expect
        .poll(async () => {
          const [paperclip, action] = await Promise.all([trigger.boundingBox(), add.boundingBox()]);
          if (!paperclip || !action) return Infinity;
          return Math.abs(paperclip.y + paperclip.height / 2 - action.y - action.height / 2);
        })
        .toBeLessThanOrEqual(1);
      await expect(manager).toBeHidden();
      await trigger.click();
      await expect(manager.getByText("Comment files", { exact: true })).toBeVisible();
      await expect(manager.getByText("Attach screenshots or files to this comment.")).toBeVisible();
      const chooser = testPage.waitForEvent("filechooser");
      await manager.getByRole("button", { name: "Add files" }).click();
      await (
        await chooser
      ).setFiles({
        name: "output-evidence.txt",
        mimeType: "text/plain",
        buffer: Buffer.from(example.bytes),
      });
      await expect(manager.getByTestId("comment-attachment-row")).toHaveAttribute(
        "data-status",
        "ready",
      );
      if (example === examples[0]) {
        await form.getByTestId("comment-attachment-input").setInputFiles({
          name: "screenshot.png",
          mimeType: "image/png",
          buffer: Buffer.from(
            "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jRZkAAAAASUVORK5CYII=",
            "base64",
          ),
        });
        await expect(manager.locator('[data-status="ready"]')).toHaveCount(2);
      }
      await testPage.keyboard.press("Escape");
      await expect(manager).toBeHidden();
      await expect(form).toBeVisible();
      await expect(trigger).toBeFocused();
      await add.click();
      await expect(form).not.toBeVisible();
    }
    await testPage.getByRole("button", { name: "2 message comments", exact: true }).hover();
    await expect(testPage.getByRole("link", { name: "output-evidence.txt" })).toHaveCount(2);
    const image = testPage.getByRole("button", { name: /^Image \(\d+ B\)$/ });
    await expect(image).toBeVisible();
    await expect
      .poll(() => image.locator("img").evaluate((img) => (img as HTMLImageElement).naturalWidth))
      .toBe(1);
    await image.click();
    const viewer = testPage.getByRole("dialog", { name: "Image preview" });
    await viewer.getByRole("button", { name: "Actual size" }).click();
    await expect(viewer.getByRole("button", { name: "Actual size" })).toHaveAttribute(
      "aria-pressed",
      "true",
    );
    await testPage.keyboard.press("Escape");
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
