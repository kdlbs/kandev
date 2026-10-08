import { expect, type Page } from "@playwright/test";
import type { AgentUpdateStatus } from "../../../lib/api/domains/agent-update-api";
import type { ApiClient } from "../../helpers/api-client";
import { waitForFiniteAnimations } from "../../helpers/animations";
import { dwell, waitForHttp, watchWs } from "../../helpers/causal-waits";
import type { BackendContext } from "../../fixtures/backend";

const SUMMARY_EVENT = "system.update_available";
const SUMMARY_TIMEOUT_MS = 75_000;

export async function backendRuntimeUpdateSummary(
  page: Page,
  backend: BackendContext,
  apiClient: ApiClient,
  mobile: boolean,
) {
  if (mobile) await page.setViewportSize({ width: 390, height: 844 });

  const ws = watchWs(page);
  const initialSubscription = ws.waitForResponse("user.subscribe");
  const initialStatus = waitForHttp(page, "GET", /\/agent-update\/status$/);
  await page.goto("/");
  await Promise.all([initialSubscription, initialStatus]);

  const summaryReceived = { value: false };
  const summaryEvent = ws
    .waitForEvent(SUMMARY_EVENT, {
      timeout: SUMMARY_TIMEOUT_MS,
      where: (payload) => payload.notification_kind === "agent_runtime_summary",
    })
    .then((frame) => {
      summaryReceived.value = true;
      return frame;
    });
  const availabilityWindowCheckStartedAt = Date.now();
  const restart = Promise.resolve().then(() =>
    backend.restart({
      KANDEV_MOCK_AGENT: "true",
      KANDEV_E2E_RUNTIME_UPDATE_LATEST_VERSION: "99.0.0",
    }),
  );
  const reconnectSubscription = ws.waitForResponse("user.subscribe", {
    timeout: 30_000,
    timeoutAfter: restart,
  });
  await Promise.all([restart, reconnectSubscription]);

  const outcomeEvent = ws.waitForEvent(SUMMARY_EVENT, {
    timeout: 15_000,
    where: (payload) => payload.runtime_update_status === "failed",
  });
  const startResponse = await apiClient.rawRequest("POST", "/api/v1/e2e/runtime-updates/startup", {
    agent_name: "gemini",
    runtime_id: "npm:@google/gemini-cli",
    display_name: "Gemini",
    previous_version: "1.0.0",
    target_version: "99.0.0",
  });
  expect(startResponse.ok).toBe(true);

  const outcome = await outcomeEvent;
  expect(outcome.payload.runtime_update_status).toBe("failed");
  const outcomeToast = page
    .getByTestId("toast-message")
    .filter({ hasText: "Gemini runtime update failed" });
  await expect(outcomeToast).toHaveCount(1);

  const statusResponse = await apiClient.rawRequest("GET", "/api/v1/agent-update/status");
  expect(statusResponse.ok).toBe(true);
  const { statuses } = (await statusResponse.json()) as { statuses: AgentUpdateStatus[] };
  const available = statuses.filter(
    (status) =>
      status.available &&
      status.enabled &&
      status.check_state === "update_available" &&
      status.latest_version === "99.0.0",
  );
  expect(
    available.length,
    JSON.stringify(
      statuses.map(
        ({ agent_name, runtime_id, available, enabled, latest_version, check_state }) => ({
          agent_name,
          runtime_id,
          available,
          enabled,
          latest_version,
          check_state,
        }),
      ),
    ),
  ).toBeGreaterThan(1);

  if (Date.now() - availabilityWindowCheckStartedAt < 30_000) {
    expect(summaryReceived.value).toBe(false);
    await expect(
      page.getByTestId("toast-message").filter({ hasText: /agent runtime updates available/ }),
    ).toHaveCount(0);
  }

  const summary = await summaryEvent;
  const members = summary.payload.runtime_updates as Array<{
    occurrence_id: string;
    agent_name: string;
    runtime_id: string;
    version: string;
  }>;
  expect(members).toHaveLength(available.length);
  expect(new Set(members.map((member) => member.occurrence_id)).size).toBe(members.length);
  expect(members.every((member) => member.version === "99.0.0")).toBe(true);

  const summaryToast = page
    .getByTestId("toast-message")
    .filter({ hasText: `${members.length} agent runtime updates available` });
  await expect(summaryToast).toHaveCount(1);
  await waitForFiniteAnimations(summaryToast);
  const review = summaryToast.getByRole("link", { name: "Review updates" });
  if (mobile) expect((await review.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  const box = (await summaryToast.boundingBox())!;
  expect(box.x).toBeGreaterThanOrEqual(0);
  expect(box.x + box.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await review.click();
  await expect(page).toHaveURL(/settings\/agents#runtime-updates$/);
  await expect(page.locator("#runtime-updates > details")).toHaveAttribute("open");

  const reloadSubscription = ws.waitForResponse("user.subscribe", { timeout: 15_000 });
  const reloadStatus = waitForHttp(page, "GET", /\/agent-update\/status$/);
  const replayedSummary = ws
    .waitForEvent(SUMMARY_EVENT, {
      timeout: 45_000,
      where: (payload) => payload.notification_kind === "agent_runtime_summary",
    })
    .then(
      () => true,
      () => false,
    );
  await page.reload();
  await Promise.all([reloadSubscription, reloadStatus]);
  await dwell(
    page,
    31_000,
    "negative-assertion",
    "the reloaded client replays availability through the controller, but durable per-member claims suppress a second summary",
  );
  expect(await replayedSummary).toBe(false);
  await expect(
    page.getByTestId("toast-message").filter({ hasText: /agent runtime updates available/ }),
  ).toHaveCount(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth > innerWidth)).toBe(false);
}
