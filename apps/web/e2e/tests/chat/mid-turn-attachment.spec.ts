import fs from "node:fs";
import path from "node:path";
import { test, expect } from "../../fixtures/test-base";
import { seedRunningGeneratingSession } from "../../helpers/generating-session";
import { typeWhileBusy } from "../../helpers/type-while-busy";

// Regression coverage for AC-TASKS-PROMPT-ATTACHMENTS-001.6: an attachment sent
// into a turn that is already generating must be materialized into the session
// before the agent is prompted.
//
// The steer route dispatched the prompt without materializing, so the agent
// received a descriptor whose bytes were never written into the session and the
// attachment silently read as empty. The observable proof is the materialized
// file on disk: the UI looks identical either way, because the descriptor is
// persisted on the user message whether or not the bytes ever landed.

const ATTACHMENT_NAME = "mid-turn-evidence.txt";

// Locates the materialized attachment beneath any session's
// `.kandev/attachments/<acp-session-id>/` directory. The ACP session id is not
// exposed to the browser, so the file is found by name rather than by path.
function findMaterializedAttachment(root: string, name: string): string | null {
  const stack: string[] = [root];
  while (stack.length > 0) {
    const dir = stack.pop();
    if (dir === undefined) continue;
    let entries: fs.Dirent[];
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true });
    } catch {
      continue; // A worktree can disappear under us mid-scan.
    }
    for (const entry of entries) {
      const full = path.join(dir, entry.name);
      if (entry.isDirectory()) {
        if (entry.name === "node_modules" || entry.name === ".git") continue;
        stack.push(full);
      } else if (entry.isFile() && entry.name === name && full.includes(".kandev")) {
        return full;
      }
    }
  }
  return null;
}

test.describe.serial("mid-turn attachment delivery", () => {
  test.describe.configure({ retries: 1 });

  test.beforeAll(async ({ backend }) => {
    await backend.restart({ KANDEV_FEATURES_CLAUDE_MID_TURN_STEERING: "true" });
    const response = await fetch(`${backend.baseUrl}/api/v1/features`);
    expect(response.ok).toBeTruthy();
    expect(await response.json()).toMatchObject({ claudeMidTurnSteering: true });
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("sends an attachment into a generating turn", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(120_000);
    const { session } = await seedRunningGeneratingSession(
      testPage,
      apiClient,
      seedData,
      "Mid-turn attachment",
    );

    fs.mkdirSync(testInfo.outputDir, { recursive: true });
    const attachmentPath = path.join(testInfo.outputDir, ATTACHMENT_NAME);
    fs.writeFileSync(attachmentPath, "mid-turn attachment bytes");

    await testPage.locator('input[type="file"]').setInputFiles(attachmentPath);
    await expect(testPage.getByText(ATTACHMENT_NAME, { exact: true })).toBeVisible({
      timeout: 10_000,
    });

    const editor = session.activeChat().locator(".tiptap.ProseMirror:visible");
    await typeWhileBusy(testPage, editor, "read the attached file");
    await testPage.getByTestId("submit-message-button").click();

    // The user message is persisted immediately. The file assertion below
    // proves that the claimed bytes reached the session working copy.
    await expect(
      session.activeChat().getByTestId("user-message-bubble").filter({
        hasText: "read the attached file",
      }),
    ).toBeVisible({ timeout: 15_000 });

    // The bytes reached the session. Before the fix this file never existed,
    // because the steer route dispatched without materializing.
    await expect
      .poll(() => findMaterializedAttachment(backend.tmpDir, ATTACHMENT_NAME) !== null, {
        timeout: 20_000,
        message: "Wait for the attachment to be materialized into the session",
      })
      .toBe(true);

    const materialized = findMaterializedAttachment(backend.tmpDir, ATTACHMENT_NAME);
    if (materialized === null) throw new Error("Materialized attachment disappeared");
    expect(fs.readFileSync(materialized, "utf8")).toBe("mid-turn attachment bytes");
  });
});
