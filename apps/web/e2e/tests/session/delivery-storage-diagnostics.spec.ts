import { expect, test } from "../../fixtures/test-base";
import { deliveryStorageFailureScenario } from "../../helpers/delivery-storage-diagnostics-ui";

deliveryStorageFailureScenario();

test("shows a live 90% advisory above the composer and clears below 85%", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Storage advisory", {
    workflow_id: seedData.workflowId,
    workflow_step_id: seedData.startStepId,
    agent_profile_id: seedData.agentProfileId,
    repository_ids: [seedData.repositoryId],
  });
  const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
    state: "WAITING_FOR_INPUT",
    agentProfileId: seedData.agentProfileId,
  });
  let notify: ((payload: Record<string, unknown>) => void) | undefined;
  await testPage.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    notify = (payload) =>
      socket.send(
        JSON.stringify({
          id: `storage-${payload.revision}`,
          type: "notification",
          action: "task_session.runtime_storage_changed",
          payload,
        }),
      );
    socket.onMessage((data) => server.send(data));
    server.onMessage((data) => socket.send(data));
  });

  await testPage.goto(`/t/${task.id}`);
  await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
  const editor = testPage.getByTestId("chat-input-editor");
  await expect(editor).toHaveAttribute("contenteditable", "true");
  await editor.fill("Keep this draft while storage guidance changes");
  const base = {
    task_id: task.id,
    session_id: sessionId,
    location: "local_runtime",
    storage_kind: "journal",
    total_bytes: 1_000_000_000,
    available_bytes: 100_000_000,
    observed_at: new Date().toISOString(),
    epoch: 1,
  };
  notify?.({ ...base, state: "warning", revision: 1 });
  const notice = testPage.getByTestId("session-delivery-storage-notice");
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Disk 90% used");
  await expect(testPage.getByTestId("session-delivery-storage-settings")).toHaveAttribute(
    "href",
    "/settings/system/storage",
  );
  await testPage.getByTestId("session-delivery-storage-details").click();
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Free space on this host",
  );
  await prCapture.screenshot(`delivery-storage-advisory-${testInfo.project.name}`, {
    caption: "Live disk advisory above the composer while work continues.",
  });
  await expect(editor).toHaveText("Keep this draft while storage guidance changes");

  const stalePressure = new Date(Date.now() - 120_000).toISOString();
  notify?.({
    ...base,
    state: "normal",
    available_bytes: 500_000_000,
    observed_at: new Date().toISOString(),
    pressure_observed_at: stalePressure,
    pressure_age_seconds: 120,
    write_pressure: true,
    failure_storage: {
      reason: "disk_quota_exceeded",
      location: "local_runtime",
      observed_at: stalePressure,
    },
    revision: 2,
  });
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Last reported");
  await expect(notice).toContainText("Delivery is blocked: storage quota exhausted");
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Pressure observed",
  );
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Capacity observed",
  );
  await expect(editor).toHaveText("Keep this draft while storage guidance changes");
  await prCapture.screenshot(`delivery-storage-pressure-stale-${testInfo.project.name}`, {
    caption: "Old pressure stays last reported while capacity is fresh.",
  });

  notify?.({
    ...base,
    state: "warning",
    available_bytes: 100_000_000,
    pressure_observed_at: new Date().toISOString(),
    pressure_age_seconds: 0,
    revision: 3,
  });
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Disk 90% used");
  await expect(notice).not.toContainText("Delivery is blocked");
  await expect(editor).toHaveText("Keep this draft while storage guidance changes");

  notify?.({ ...base, state: "warning", available_bytes: 120_000_000, revision: 4 });
  await expect(notice).toHaveCount(1);
  notify?.({ ...base, state: "normal", available_bytes: 200_000_000, revision: 5 });
  await expect(notice).toHaveCount(0);

  notify?.({
    ...base,
    state: "normal",
    available_bytes: 500_000_000,
    write_pressure: true,
    producer_paused: true,
    delivery_state: "delivery_failed",
    failure_storage: {
      reason: "disk_quota_exceeded",
      location: "local_runtime",
      observed_at: new Date().toISOString(),
    },
    revision: 6,
  });
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Delivery is blocked: storage quota exhausted");
  await expect(notice).not.toContainText("Work continues");
  notify?.({ ...base, state: "normal", available_bytes: 500_000_000, revision: 7 });
  await expect(notice).toHaveCount(0);
  await expect(testPage.getByTestId("chat-input-area")).toBeVisible();

  await testPage.reload();
  await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
  await expect(notice).toHaveCount(0);
  notify?.({ ...base, state: "normal", available_bytes: 500_000_000, revision: 8 });
  await expect(notice).toHaveCount(0);
});
