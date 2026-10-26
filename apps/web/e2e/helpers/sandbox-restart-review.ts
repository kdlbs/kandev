import { expect, test } from "../fixtures/test-base";
import type { Page } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import type { SandboxRevision } from "@/lib/types/sandbox";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { waitForFiniteAnimations } from "./pr-capture";

type RecordedRequest = { action: string; payload: Record<string, unknown> };
async function recordRestart(page: Page, sessionId: string, writes: RecordedRequest[]) {
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((data) => {
      if (typeof data !== "string") return server.send(data);
      for (const part of data.split("\n").filter(Boolean)) {
        const frame = JSON.parse(part);
        if (
          frame.type === "request" &&
          frame.payload?.session_id === sessionId &&
          ["session.stop", "session.launch"].includes(frame.action)
        ) {
          writes.push({ action: frame.action, payload: frame.payload });
          socket.send(
            JSON.stringify({
              id: frame.id,
              type: "response",
              action: frame.action,
              payload: {
                success: true,
                session_id: sessionId,
                task_id: frame.payload.task_id,
                state: "STARTING",
              },
            }),
          );
        } else server.send(part);
      }
    });
    server.onMessage((data) => socket.send(data));
  });
}
function revision(): SandboxRevision {
  return {
    id: "ui-revision",
    number: 7,
    schema_version: 1,
    digest: "c".repeat(64),
    operator_id: "operator",
    org_id: "org",
    created_at: "2026-10-26T20:00:00Z",
    revoked: false,
    policy: {
      id: "work",
      workspace_id: "",
      name: "Captured policy",
      installation_id: "ui-install",
      provider_key: "fixture-provider",
      grants: {
        filesystem: [{ reference: "work_dir", writable: true }],
        network: [],
        kandev_mcp: true,
      },
      sources: [],
    },
  };
}
async function openRestart(page: Page, sessionId: string, mobile: boolean) {
  if (mobile) {
    await page.getByTestId("mobile-sessions-pill").click();
    await page
      .getByTestId(`mobile-session-row-${sessionId}`)
      .getByRole("button", { name: "Session actions", exact: true })
      .click();
  } else await page.getByTestId(`session-tab-${sessionId}`).click({ button: "right" });
  await page.getByRole("menuitem", { name: "Restart session", exact: true }).click();
  const review = page.getByRole("dialog", { name: "Review sandbox restart", exact: true });
  await expect(review).toBeVisible();
  return review;
}
export function sandboxRestartReviewScenario() {
  test("reviews a changed sandbox policy before stopping and preserves the frozen option", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Review sandbox restart", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const writes: RecordedRequest[] = [];
    await recordRestart(testPage, sessionId, writes);
    const captured = revision();
    const current = {
      ...captured,
      id: "current",
      number: 8,
      digest: "b".repeat(64),
      policy: {
        ...captured.policy,
        name: "Current approved policy " + "long-name-".repeat(12),
        grants: {
          ...captured.policy.grants,
          filesystem: [
            ...captured.policy.grants.filesystem,
            { path: "/test-fixture/" + "long-path/".repeat(15), writable: false },
          ],
          network: [{ host: "approved.example.test", port: 443, protocol: "https" as const }],
        },
      },
    };
    let digest = "e".repeat(64);
    await testPage.route("**/api/v1/sandbox/**", async (route) => {
      if (new URL(route.request().url()).pathname.endsWith("/preview")) {
        await route.fulfill({
          json: {
            resolved: {
              revision: current,
              ceiling: { ...current, id: "ceiling", digest: "d".repeat(64) },
              executor_id: "ui-executor",
              binding_version: 2,
              selection_source: "executor",
              bindings: [],
            },
            eligible: [current],
            selection_digest: digest,
          },
        });
      } else if (route.request().url().includes("/revisions/"))
        await route.fulfill({ json: captured });
      else await route.fulfill({ status: 404, json: {} });
    });
    await testPage.goto(`/t/${task.id}`);
    await enableSandboxSettingsFixture(testPage);
    await projectExecutionFixture(testPage, sessionId);
    let review = await openRestart(testPage, sessionId, mobile);
    await expect(
      review.getByText("The current policy differs from the captured policy."),
    ).toBeVisible();
    expect(writes).toHaveLength(0);
    const keep = review.getByRole("button", { name: "Keep captured and restart", exact: true });
    const useCurrent = review.getByRole("button", {
      name: "Use current policy and restart",
      exact: true,
    });
    await expect(useCurrent).toBeEnabled();
    await expect(
      review.getByText("https://approved.example.test:443", { exact: true }),
    ).toBeVisible();
    const capturedPermissions = review.locator("details").first();
    await expect(capturedPermissions).not.toHaveAttribute("open");
    await capturedPermissions.locator("summary").click();
    await expect(capturedPermissions).toHaveAttribute("open");
    await capturedPermissions.locator("summary").click();
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await expect.poll(async () => Math.round((await review.boundingBox())!.width)).toBe(width);
        expect((await keep.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        expect((await useCurrent.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        expect(
          (await review.getByRole("button", { name: "Back", exact: true }).boundingBox())!.height,
        ).toBeGreaterThanOrEqual(44);
        await assertNoDocumentHorizontalOverflow(testPage, `restart at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
      await expect(
        testPage.getByRole("dialog", { name: "Sessions", exact: true }),
      ).not.toBeVisible();
    } else expect(Math.round((await keep.boundingBox())!.height)).toBe(28);
    const body = review.getByTestId("restart-review-body");
    expect(await body.evaluate((el) => getComputedStyle(el).overscrollBehaviorY)).toBe("contain");
    await assertNoDocumentHorizontalOverflow(testPage, "restart review");
    if (prCapture.capturing) await waitForFiniteAnimations(review);
    await prCapture.screenshot(mobile ? "restart-policy-phone" : "restart-policy-desktop", {
      caption: "Review captured and current approved permissions before restarting the session.",
    });
    await review.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(review).not.toBeVisible();
    await expect(testPage.getByTestId(`execution-control-${sessionId}`)).toBeFocused();
    expect(writes).toHaveLength(0);
    review = await openRestart(testPage, sessionId, mobile);
    await expect(
      review.getByRole("button", { name: "Use current policy and restart", exact: true }),
    ).toBeEnabled();
    digest = "f".repeat(64);
    await review
      .getByRole("button", { name: "Use current policy and restart", exact: true })
      .click();
    await expect(review.getByRole("alert")).toContainText("The session or policy changed.");
    expect(writes).toHaveLength(0);
    await review
      .getByRole("button", { name: "Use current policy and restart", exact: true })
      .click();
    await expect(review).not.toBeVisible();
    await expect.poll(() => writes.length).toBe(2);
    expect(writes).toEqual([
      { action: "session.stop", payload: { session_id: sessionId } },
      {
        action: "session.launch",
        payload: {
          task_id: task.id,
          intent: "resume",
          session_id: sessionId,
          sandbox_restart_review: { attempt_id: "ui-attempt", selection_digest: digest },
        },
      },
    ]);
    await testPage.evaluate(() => {
      const store = (window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } })
        .__KANDEV_E2E_STORE__;
      const state = store.getState();
      state.setAuthState({ ...state.auth, user: { ...state.auth.user!, role: "member" } });
    });
    review = await openRestart(testPage, sessionId, mobile);
    await expect(review.getByText("Only an operator can select a different policy.")).toBeVisible();
    await expect(
      review.getByRole("button", { name: "Use current policy and restart", exact: true }),
    ).toBeDisabled();
    await review.getByRole("button", { name: "Keep captured and restart", exact: true }).click();
    await expect.poll(() => writes.length).toBe(4);
    expect(writes[3].payload).toEqual({
      task_id: task.id,
      intent: "resume",
      session_id: sessionId,
    });
  });
}
