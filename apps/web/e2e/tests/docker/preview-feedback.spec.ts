import { test, expect } from "../../fixtures/docker-test-base";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";
import { chooseCapture, saveDraft } from "../preview/preview-feedback-helpers";

const PREVIEW_PORT = 4173;
const PREVIEW_HTML = `<!doctype html>
<html>
  <head><meta charset="utf-8"><title>Docker preview fixture</title></head>
  <body><main><h1>Container preview</h1><button id="deploy">Deploy preview</button></main></body>
</html>`;

function dockerPreviewScript(): string {
  const source = [
    'const http = require("node:http")',
    `const html = ${JSON.stringify(PREVIEW_HTML)}`,
    'const server = http.createServer((_request, response) => { response.setHeader("content-type", "text/html; charset=utf-8"); response.end(html) })',
    `server.listen(${PREVIEW_PORT}, "0.0.0.0", () => console.log("http://localhost:${PREVIEW_PORT}"))`,
  ].join(";");
  return `node -e '${source}'`;
}

test.describe("Docker web preview feedback", () => {
  test.describe.configure({ retries: 1, timeout: 180_000 });

  test("annotates a container-served page and delivers the feedback", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await apiClient.updateRepository(seedData.repositoryId, {
      dev_script: dockerPreviewScript(),
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Docker Preview Feedback",
        seedData.agentProfileId,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
          executor_profile_id: seedData.dockerExecutorProfileId,
        },
      );
      await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for Docker preview task");

      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 60_000 });
      await testPage.getByTestId("dev-server-preview-toggle-center").click();
      await expect(session.browserPanel).toBeVisible({ timeout: 30_000 });
      await session.browserAddressInput.fill(`http://localhost:${PREVIEW_PORT}`);
      await session.browserAddressInput.press("Enter");

      const frame = session.browserPanel.frameLocator("iframe");
      await expect(frame.locator("#deploy")).toBeVisible({ timeout: 60_000 });
      await chooseCapture(testPage, "Select element", frame);
      await frame.locator("#deploy").click();
      await saveDraft(testPage, "Clarify the result of this container action");

      await session.clickSessionChatTab();
      await session.sendMessageViaButton("Apply the container preview feedback");
      const message = session
        .activeChat()
        .getByTestId("user-message-bubble")
        .filter({ hasText: "Apply the container preview feedback" });
      await expect(message).toContainText("Web Preview Feedback", { timeout: 30_000 });
      await expect(message).toContainText("Clarify the result of this container action");
      await expect(message).toContainText("http://localhost:4173");
    } finally {
      await apiClient.updateRepository(seedData.repositoryId, { dev_script: "" });
    }
  });
});
