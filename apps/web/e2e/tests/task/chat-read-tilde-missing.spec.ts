// Regression guard for "~/"-home agent read links. OMP emits read paths like
// "~/.kandev/x/foo.spec.ts:760-860"; the chat link strips the selector and opens
// the bare "~/..." path. When that file does not exist, the backend must report
// the failure against the expanded $HOME location, NOT by gluing the tilde onto
// the workspace root (".../<workspace>/~/...: no such file"), which reads as a
// mangled path. Companion to chat-read-multi-file.spec.ts (selector splitting)
// and the Go unit coverage in workspace_tracker_test.go.
import { test, expect } from "../../fixtures/test-base";
import { createStandardProfile } from "../../helpers/git-helper";
import { SessionPage } from "../../pages/session-page";

test.describe("Chat tilde read links", () => {
  test("missing ~/ read link fails with a clean, non-mangled error", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const suffix = `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
    // A "~/"-home path whose target exists nowhere (neither under the workspace
    // nor under $HOME). The bug glued the tilde onto the workspace root; the fix
    // surfaces the expanded-home location instead.
    const linkPath = `~/kandev-e2e-missing-${suffix}/account-mapping.spec.ts`;

    const profile = await createStandardProfile(apiClient, `tilde-read-${Date.now()}`);
    // `e2e:message(...)` emits a single non-activity text message, so the seeded
    // read card is the only activity message and renders as a standalone card.
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Chat Tilde Read",
      profile.id,
      {
        description: 'e2e:message("ready")',
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("createTaskWithAgent did not return a session_id");

    // Seed a read card whose file_path is a "~/"-home shorthand with a line
    // selector, exactly as the backend stores an omp read of a home-path file.
    await apiClient.seedSessionMessage(task.session_id, {
      type: "tool_read",
      content: "Read file",
      metadata: {
        status: "complete",
        tool_call_id: "tc-tilde-missing-read",
        normalized: {
          read_file: {
            file_path: `${linkPath}:760-860`,
            output: { content: "", line_count: 0 },
          },
        },
      },
    });

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 45_000 });

    // The link carries the bare home path (selector stripped by splitReadFiles).
    const chat = session.activeChat();
    const link = chat.locator(`button[title="${linkPath}"]`);
    await expect(link).toBeVisible({ timeout: 15_000 });

    await link.click();

    // The open fails (the file is missing). The error toast must report a clean
    // "file not found" naming the expanded $HOME path — never the mangled
    // ".../<workspace>/~/..." join, and never an alarming "path traversal".
    const toast = testPage.getByTestId("toast-message").filter({ hasText: "Failed to open file" });
    await expect(toast).toBeVisible({ timeout: 10_000 });
    const text = (await toast.textContent()) ?? "";
    expect(text, `toast must not glue ~ onto the workspace root: ${text}`).not.toContain("/~/");
    expect(text.toLowerCase(), `toast should report a missing file: ${text}`).toContain(
      "not found",
    );
    expect(text.toLowerCase(), `missing file must not read as traversal: ${text}`).not.toContain(
      "path traversal",
    );
  });
});
