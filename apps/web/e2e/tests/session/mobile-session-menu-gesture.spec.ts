import { writeFile } from "node:fs/promises";
import { test, expect } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
import { attachGatewayTrafficCapture } from "../../helpers/ws-traffic";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { SessionPage } from "../../pages/session-page";

// @covers AC-UI-MOBILE-TASK-NAVIGATION-001.3, AC-UI-MOBILE-TASK-NAVIGATION-001.11, AC-UI-MOBILE-TASK-NAVIGATION-001.12
test("mobile: opening a native session menu owns the completed touch gesture", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Session menu touch ownership",
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("Missing primary session");
  await expect
    .poll(async () => (await apiClient.listTaskSessions(task.id)).sessions[0]?.state)
    .toBe("WAITING_FOR_INPUT");
  const secondary = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
    repositoryId: seedData.repositoryId,
    sessionId: `menu-${task.id}`,
    startedAt: "2026-01-01T00:01:00Z",
  });
  await apiClient.renameSession(secondary.session_id, "Secondary conversation");
  const traffic = attachGatewayTrafficCapture(testPage);
  const requests: Array<{
    phase: string;
    action: string;
    payload: Record<string, unknown>;
    automaticFocus: boolean;
  }> = [];
  let phase = "navigation";
  let expectedFocusSession = task.session_id;
  let stimulusStart = 0;
  testPage.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", ({ payload }) => {
      const request = JSON.parse(typeof payload === "string" ? payload : payload.toString());
      if (
        ![
          "session.stop",
          "session.launch",
          "session.resume",
          "session.restart",
          "session.delete",
          "session.set_primary",
        ].includes(request.action)
      )
        return;
      const body = request.payload ?? {};
      // use-session-resumption sends this exact shape for the visible session.
      const automaticFocus =
        request.action === "session.launch" &&
        JSON.stringify(Object.keys(body).sort()) ===
          JSON.stringify(["activation_source", "intent", "session_id", "task_id"]) &&
        body.activation_source === "session_focus" &&
        body.intent === "resume" &&
        body.task_id === task.id &&
        body.session_id === expectedFocusSession;
      requests.push({ phase, action: request.action, payload: body, automaticFocus });
    });
  });
  const mutations = () =>
    requests.slice(stimulusStart).filter((request) => !request.automaticFocus);
  try {
    await testPage.goto(`/t/${task.id}`);
    await new SessionPage(testPage).waitForLoad();
    stimulusStart = requests.length;
    phase = "picker selection";
    const pill = testPage.getByTestId("mobile-task-layout").getByTestId("mobile-sessions-pill");
    await pill.tap();
    const sheet = testPage.getByRole("dialog", { name: "Sessions", exact: true });
    const row = sheet.getByTestId(`mobile-session-row-${secondary.session_id}`);
    expectedFocusSession = secondary.session_id;
    await row.tap();
    await expect(sheet).not.toBeVisible();
    await expect(pill).toContainText("Secondary conversation");
    await pill.tap();
    await expect(row).toHaveAttribute("aria-current", "true");
    expect(mutations()).toEqual([]);
    const trigger = row.getByRole("button", { name: "Session actions", exact: true });
    const menu = testPage.getByRole("menu");
    await expect(trigger).toHaveCount(1);
    const triggerBox = (await trigger.boundingBox())!;
    const rowBox = (await row.boundingBox())!;
    phase = "opening trigger touch";
    await trigger.tap();
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "Completing the menu opening touch has no domain mutation acknowledgement",
    );
    expect(mutations(), "opening touch must not issue any session mutation").toEqual([]);
    // The update bridge can show this informational notice independently of session actions.
    const unexpectedToasts = testPage
      .getByTestId("toast-message")
      .filter({ hasNotText: "Kandev update available" });
    await expect(unexpectedToasts).toHaveCount(0);
    await expect(menu).toBeVisible();
    await waitForFiniteAnimations(menu);
    expect(triggerBox.height).toBeGreaterThanOrEqual(44);
    expect(triggerBox.width).toBeGreaterThanOrEqual(44);
    expect(triggerBox.x).toBeGreaterThanOrEqual(rowBox.x);
    expect(triggerBox.x + triggerBox.width).toBeLessThanOrEqual(rowBox.x + rowBox.width);
    const menuBox = (await menu.boundingBox())!;
    expect(menuBox.x).toBeGreaterThanOrEqual(0);
    expect(menuBox.y).toBeGreaterThanOrEqual(0);
    expect(menuBox.x + menuBox.width).toBeLessThanOrEqual(testPage.viewportSize()!.width);
    expect(menuBox.y + menuBox.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
    await assertNoDocumentHorizontalOverflow(testPage, "native session action menu");
    await testPage.screenshot({ path: test.info().outputPath("session-menu-phone.png") });

    phase = "Escape dismissal";
    await testPage.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);
    await expect(sheet).toBeVisible();
    await expect(trigger).toBeFocused();
    phase = "keyboard opening";
    await testPage.keyboard.press("Enter");
    await expect(menu).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);
    await expect(row).toHaveAttribute("aria-current", "true");
    phase = "hybrid mouse opening";
    await trigger.click();
    await expect(menu).toBeVisible();
    await testPage.keyboard.press("Escape");
    await expect(menu).toHaveCount(0);
    phase = "repeated touch opening and outside dismissal";
    await trigger.tap();
    await expect(menu).toBeVisible();
    await testPage.touchscreen.tap(16, 16);
    await expect(menu).toHaveCount(0);
    await expect(sheet).toBeVisible();
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "Menu dismissal has no domain mutation acknowledgement",
    );
    expect(mutations()).toEqual([]);
    await expect(unexpectedToasts).toHaveCount(0);

    await trigger.tap();
    const setPrimary = menu.getByRole("menuitem", { name: "Set as Primary", exact: true });
    await expect(setPrimary).toBeEnabled();
    await waitForFiniteAnimations(menu);
    expect((await setPrimary.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    phase = "explicit primary item touch";
    await setPrimary.tap();
    await expect
      .poll(async () =>
        (await apiClient.listTaskSessions(task.id)).sessions
          .filter((session) => session.is_primary)
          .map((session) => session.id),
      )
      .toEqual([secondary.session_id]);
    await expect(menu).toHaveCount(0);
    await dwell(
      testPage,
      300,
      "negative-assertion",
      "The explicit item tap must not duplicate the session mutation",
    );
    expect(mutations()).toEqual([
      {
        phase: "explicit primary item touch",
        action: "session.set_primary",
        payload: { session_id: secondary.session_id },
        automaticFocus: false,
      },
    ]);
    await expect(row).toHaveAttribute("aria-current", "true");
    phase = "reload persistence";
    await testPage.reload();
    await new SessionPage(testPage).waitForLoad();
    expect(
      (await apiClient.listTaskSessions(task.id)).sessions.find(
        (session) => session.id === secondary.session_id,
      )?.is_primary,
    ).toBe(true);
  } finally {
    const historyPath = test.info().outputPath("gateway-history.json");
    await writeFile(
      historyPath,
      JSON.stringify({ stimulusStart, requests, frames: traffic.frames }, null, 2),
    );
    await test
      .info()
      .attach("gateway-history.json", { path: historyPath, contentType: "application/json" });
  }
});
