import { test, expect } from "../../fixtures/test-base";
import { GitHelper, makeGitEnv } from "../../helpers/git-helper";
import { waitForSessionDone, waitForWorkspacePath } from "../../helpers/session";
import { watchWs } from "../../helpers/causal-waits";
import { SessionPage } from "../../pages/session-page";
import type { GitStatusData } from "../../../lib/types/git-events";
import type { ExecutePromptRequest } from "@/lib/api/domains/utility-api";

const CHANGED_FILE_COUNT = 205;
const LISTED_FILE_LIMIT = 200;
const changedFile = (index: number) => `enhance-context-${String(index).padStart(3, "0")}.ts`;

test.describe("Enhance prompt in a task session", () => {
  // @covers AC-AGENTS-UTILITY-AGENT-PROFILES-002.1
  test("sends the changed file names, capped, and never the session diff", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }) => {
    test.setTimeout(120_000);
    const { settings } = await apiClient.getUserSettings();
    const baseline = settings as {
      default_utility_agent_id?: string;
      default_utility_agent_profile_id?: string;
    };
    await apiClient.saveUserSettings({
      default_utility_agent_id: "",
      default_utility_agent_profile_id: seedData.agentProfileId,
    });
    let createdIn: GitHelper | null = null;
    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Enhance session context",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      await waitForSessionDone(apiClient, task.id, task.session_id!, "Enhance fixture settled");
      const workspace = await waitForWorkspacePath(apiClient, task.id, task.session_id!);
      const git = new GitHelper(workspace, makeGitEnv(backend.tmpDir));
      createdIn = git;

      let executeBody: ExecutePromptRequest | null = null;
      await testPage.route("**/api/v1/utility/execute", async (route) => {
        executeBody = route.request().postDataJSON() as ExecutePromptRequest;
        await route.fulfill({
          status: 200,
          contentType: "application/json",
          body: JSON.stringify({
            success: true,
            call_id: "enhance-session-call",
            response: "Synthetic enhanced session prompt.",
          }),
        });
      });

      const gateway = watchWs(testPage);
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();

      const allChangesPublished = gateway.waitForEvent("session.git.event", {
        where: (payload) => {
          const files = (payload.status as GitStatusData | undefined)?.files ?? {};
          return (
            payload.session_id === task.session_id &&
            payload.type === "status_update" &&
            Object.keys(files).filter((name) => name.startsWith("enhance-context-")).length ===
              CHANGED_FILE_COUNT
          );
        },
      });
      for (let index = 0; index < CHANGED_FILE_COUNT; index++) {
        git.createFile(changedFile(index), `export const value${index} = ${index};\n`);
      }
      await allChangesPublished;

      await session.fillMessage("Synthetic prompt to enhance.");
      const enhance = session.activeChat().getByTestId("enhance-prompt-button");
      await expect(enhance).toBeEnabled();
      await enhance.click();

      await expect
        .poll(() => executeBody?.user_prompt ?? null)
        .toBe("Synthetic prompt to enhance.");
      const body = executeBody as ExecutePromptRequest | null;
      expect(body).toMatchObject({
        utility_agent_id: "builtin-enhance-prompt",
        session_id: task.session_id,
      });
      expect(body).not.toHaveProperty("git_diff");
      const listed = (body?.changed_files ?? "").split("\n");
      expect(listed).toHaveLength(LISTED_FILE_LIMIT + 1);
      expect(listed.filter((line) => line.startsWith("enhance-context-")).length).toBeGreaterThan(
        0,
      );
      const remainder = /^\.\.\. and (\d+) more files$/.exec(listed[LISTED_FILE_LIMIT]);
      expect(Number(remainder?.[1])).toBeGreaterThanOrEqual(CHANGED_FILE_COUNT - LISTED_FILE_LIMIT);
    } finally {
      for (let index = 0; index < CHANGED_FILE_COUNT; index++) {
        createdIn?.deleteFile(changedFile(index));
      }
      await apiClient.saveUserSettings({
        default_utility_agent_id: baseline.default_utility_agent_id ?? "",
        default_utility_agent_profile_id: baseline.default_utility_agent_profile_id ?? "",
      });
    }
  });
});
