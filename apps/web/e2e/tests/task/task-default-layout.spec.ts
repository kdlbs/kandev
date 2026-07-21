import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

// Regression: entering a task must lay out the DEFAULT layout horizontally —
// chat/agent in the center (left), files/changes + terminal in a right column.
// A bug stacked them vertically (chat on top, files/changes in the middle,
// terminal at the bottom): the chat→session-tab swap removed the "chat"
// placeholder before adding the session panel, which destroyed the center
// group and collapsed the horizontal split into a single vertical column.
test.describe("Task default layout shape", () => {
  test("repo-less tasks keep every panel in the built-in default layout", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Repo-less Default Layout",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();

    await expect
      .poll(
        () =>
          testPage.evaluate(() => {
            type Api = { panels: Array<{ id: string }> };
            const api = (window as unknown as { __dockviewApi__?: Api }).__dockviewApi__;
            return api?.panels.map((panel) => panel.id).sort() ?? [];
          }),
        { message: "Waiting for every built-in default panel" },
      )
      .toEqual(["changes", "files", `session:${task.session_id}`, "terminal-default"]);
  });

  test("entering a task is horizontal (right column beside chat, not stacked)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Default Layout Shape",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForDockviewReady();
    await testPage.waitForTimeout(500); // let the session-tab swap settle

    const container = await testPage.getByTestId("dockview-task-layout").evaluate((el) => {
      const r = el.getBoundingClientRect();
      return { left: r.x, width: r.width };
    });

    const groups = await testPage.evaluate(() => {
      const els = Array.from(document.querySelectorAll(".dv-groupview")) as HTMLElement[];
      return els.map((el) => {
        const r = el.getBoundingClientRect();
        return { x: Math.round(r.x), w: Math.round(r.width) };
      });
    });

    // The right column (files/changes + terminal) must sit beside chat — i.e.
    // at least one group starts in the right portion of the layout. If the
    // layout collapsed to a vertical stack, every group shares the same left x.
    // Normalise each group's viewport x by the container's left edge so the
    // sidebar width doesn't skew the comparison against the container width.
    const rightColumnGroups = groups.filter((g) => g.x - container.left > container.width * 0.4);
    expect(
      rightColumnGroups.length,
      `expected a right-side column; groups=${JSON.stringify(groups)} container=${JSON.stringify(container)}`,
    ).toBeGreaterThan(0);
  });
});
