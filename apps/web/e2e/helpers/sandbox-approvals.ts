import type { Page, WebSocketRoute } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import type { SandboxApproval } from "@/lib/api/domains/sandbox-approvals-api";
import { expect, test } from "../fixtures/test-base";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

async function routeApprovals(page: Page, sessionId: string) {
  let socket: WebSocketRoute | undefined;
  let connections = 0;
  let records: SandboxApproval[] = [];
  let reads = 0;
  const decisions: unknown[] = [];
  await page.routeWebSocket(/\/ws$/, (ws) => {
    socket = ws;
    connections++;
    ws.connectToServer();
  });
  await page.route("**/api/v1/sandbox/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (path === `/api/v1/sandbox/sessions/${sessionId}/approvals`) {
      await route.fulfill({ json: records });
      reads++;
    } else if (request.method() === "POST" && path.endsWith("/decision")) {
      const body = request.postDataJSON();
      decisions.push(body);
      const record = records.find((row) => path.includes(`/approvals/${row.request_id}/`))!;
      expect(body.expected_version).toBe(record.version);
      record.state = "sent";
      record.version += 3;
      record.can_decide = false;
      await route.fulfill({ json: record });
    } else {
      await route.fulfill({ status: 404, json: {} });
    }
  });
  return {
    decisions,
    get reads() {
      return reads;
    },
    get connections() {
      return connections;
    },
    async publish(rows: SandboxApproval[]) {
      records = rows;
      const payload = await page.evaluate((id) => {
        const state = (
          window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
        ).__KANDEV_E2E_STORE__.getState();
        const session = state.taskSessions.items[id];
        return {
          task_id: session.task_id,
          session_id: id,
          queue_incarnation_id: session.queue_incarnation_id,
        };
      }, sessionId);
      expect(socket).toBeTruthy();
      socket!.send(
        JSON.stringify({
          id: "approval-change",
          type: "notification",
          action: "session.sandbox_approvals_changed",
          payload,
          timestamp: Date.now(),
        }),
      );
    },
  };
}

function pendingApproval(sessionId: string, requestId: string): SandboxApproval {
  return {
    request_id: requestId,
    attempt_id: "ui-attempt",
    session_id: sessionId,
    version: 1,
    state: "pending",
    access: "read",
    target: "/workspace/" + "long-directory/".repeat(25) + "restricted.txt",
    policy_digest: "c".repeat(64),
    expires_at: "2099-01-01T00:00:00Z",
    can_decide: true,
  };
}

export function sandboxApprovalsScenario() {
  test("reviews live sandbox requests without changing the saved policy", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Sandbox approvals", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const fixture = await routeApprovals(testPage, sessionId);
    await testPage.goto(`/t/${task.id}`);
    await projectExecutionFixture(testPage, sessionId);
    const connections = fixture.connections;
    await enableSandboxSettingsFixture(testPage);
    await expect.poll(() => fixture.connections).toBeGreaterThan(connections);
    await testPage.waitForFunction(
      () =>
        (
          window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
        ).__KANDEV_E2E_STORE__.getState().connection.status === "connected",
    );
    const trigger = testPage.getByRole("button", {
      name: /^Review sandbox permissions for /,
    });
    const execution = testPage.getByTestId(`execution-control-${sessionId}`);
    await expect(trigger).not.toBeAttached();
    await expect.poll(() => fixture.reads).toBeGreaterThan(0);
    const beforeEvent = fixture.reads;
    const first = pendingApproval(sessionId, "first-request");
    await fixture.publish([first]);
    await expect(trigger).toBeVisible();
    await expect.poll(() => fixture.reads).toBeGreaterThan(beforeEvent);
    await expect(testPage.getByRole("dialog")).not.toBeVisible();
    if (mobile) {
      expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
      await assertNoDocumentHorizontalOverflow(testPage, "approval notice");
      await trigger.tap();
    } else {
      await trigger.focus();
      await testPage.keyboard.press("Enter");
    }
    const dialog = testPage.getByRole("dialog", { name: "Review sandbox permission", exact: true });
    await expect(dialog).toBeVisible();
    await expect(dialog.getByText(first.target, { exact: true })).toBeVisible();
    await expect(dialog.getByText(sessionId, { exact: true })).toBeVisible();
    await expect(dialog.getByText(/^Captured policy long-name-/)).toBeVisible();
    await expect(dialog.getByText(/does not change the saved policy/)).toBeVisible();
    await dialog.press("Enter");
    expect(fixture.decisions).toEqual([]);
    const allow = dialog.getByRole("button", { name: "Allow once", exact: true });
    await expect(allow).toBeEnabled();
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await expect.poll(async () => Math.round((await dialog.boundingBox())!.width)).toBe(width);
        expect((await allow.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await expect(allow).toBeInViewport();
        await assertNoDocumentHorizontalOverflow(testPage, `approval review at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    expect(
      await dialog
        .getByTestId("sandbox-approval-body")
        .evaluate((node) => getComputedStyle(node).overscrollBehaviorY),
    ).toBe("contain");
    await dialog.screenshot({ path: testInfo.outputPath("sandbox-approval.png") });
    await allow.click();
    await expect
      .poll(() => fixture.decisions)
      .toEqual([{ expected_version: 1, decision: "allow_once" }]);
    await expect(allow).toBeDisabled();
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(dialog).not.toBeVisible();
    await expect(trigger).not.toBeAttached();
    await expect(execution).toBeFocused();

    await fixture.publish([pendingApproval(sessionId, "second-request")]);
    await trigger.click();
    await expect(dialog).toBeVisible();
    await dialog.getByRole("button", { name: "Deny", exact: true }).click();
    await expect.poll(() => fixture.decisions.length).toBe(2);
    expect(fixture.decisions[1]).toEqual({ expected_version: 1, decision: "deny" });
    await expect(allow).toBeDisabled();
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(execution).toBeFocused();

    const third = pendingApproval(sessionId, "third-request");
    await fixture.publish([third]);
    await trigger.click();
    await expect(allow).toBeEnabled();
    await testPage.evaluate(() => {
      const state = (
        window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
      ).__KANDEV_E2E_STORE__.getState();
      state.setAuthState({ ...state.auth, user: { ...state.auth.user!, role: "member" } });
    });
    await expect(allow).toBeDisabled();
    await expect(dialog.getByRole("button", { name: "Deny", exact: true })).toBeDisabled();
    await fixture.publish([{ ...third, expires_at: "2020-01-01T00:00:00Z" }]);
    await expect(dialog.getByRole("status")).toHaveText("Expired");
    await testPage.evaluate(() => {
      const state = (
        window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
      ).__KANDEV_E2E_STORE__.getState();
      state.setConnectionStatus("disconnected");
    });
    await expect(dialog.getByText(third.target, { exact: true })).not.toBeAttached();
    await expect(allow).not.toBeAttached();
    expect(fixture.decisions).toHaveLength(2);
  });
}
