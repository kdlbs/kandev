import { test, expect, resetSeedRepositoryCheckout } from "../../fixtures/test-base";
import { createStandardProfile } from "../../helpers/git-helper";
import { waitForLatestSessionDone } from "../../helpers/session";
import { SessionPage } from "../../pages/session-page";

test("Changes automatically recovers its initial Git read without showing a false empty state", async ({
  testPage,
  apiClient,
  seedData,
  backend,
}) => {
  resetSeedRepositoryCheckout(seedData, backend.tmpDir);
  const profile = await createStandardProfile(apiClient, "Git read recovery");
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Git read recovery",
    profile.id,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  await waitForLatestSessionDone(apiClient, task.id, 1, "Wait for the Git workspace");

  let unavailable = true;
  let requests = 0;
  await testPage.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string") return server.send(message);
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part) as { type?: string; action?: string; id?: string };
        if (frame.type === "request" && frame.action === "session.git.commits") {
          requests++;
          if (unavailable) {
            socket.send(
              JSON.stringify({
                ...frame,
                type: "error",
                payload: { code: "UNAVAILABLE", message: "Temporary Git outage" },
              }),
            );
            continue;
          }
        }
        server.send(part);
      }
    });
  });

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await session.clickTab("Changes");
  await expect.poll(() => requests, { timeout: 15_000 }).toBeGreaterThan(0);
  const loading = session.changes.getByTestId("changes-panel-scroll-owner").getByRole("status");
  await expect(loading).toHaveText("Loading changes...");
  const empty = session.changes.getByText("Your changed files will appear here");
  await expect(empty).toHaveCount(0);

  const beforeRecovery = requests;
  unavailable = false;
  await expect.poll(() => requests, { timeout: 15_000 }).toBeGreaterThan(beforeRecovery);
  await expect(empty).toBeVisible();
  await expect(loading).toHaveCount(0);
});
