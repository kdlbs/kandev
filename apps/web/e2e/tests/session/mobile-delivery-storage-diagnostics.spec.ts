import { expect, test } from "../../fixtures/test-base";
import { deliveryStorageFailureScenario } from "../../helpers/delivery-storage-diagnostics-ui";

deliveryStorageFailureScenario();

test("keeps the phone advisory compact with 44px controls", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}, testInfo) => {
  const task = await apiClient.createTask(seedData.workspaceId, "Mobile storage advisory", {
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
  await editor.fill("Phone draft survives guidance updates");
  const base = {
    task_id: task.id,
    session_id: sessionId,
    location: "executor_runtime",
    executor_label: "sprites",
    storage_kind: "workspace",
    total_bytes: 1_000_000_000,
    available_bytes: 50_000_000,
    observed_at: new Date().toISOString(),
    epoch: 1,
    state: "warning",
    revision: 1,
  };
  notify?.(base);
  const notice = testPage.getByTestId("session-delivery-storage-notice");
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("sprites: Disk 95% used");
  await expect(testPage.getByTestId("session-delivery-storage-settings")).toHaveCount(0);
  const details = testPage.getByTestId("session-delivery-storage-details");
  expect((await details.boundingBox())!.height).toBeGreaterThanOrEqual(44);
  await details.click();
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Free space on sprites",
  );
  await prCapture.screenshot(`delivery-storage-advisory-${testInfo.project.name}`, {
    caption: "Phone disk advisory with executor scope, usage, and 44px controls.",
  });
  await expect(editor).toHaveText("Phone draft survives guidance updates");

  const stalePressure = new Date(Date.now() - 120_000).toISOString();
  await notify!({
    ...base,
    state: "normal",
    available_bytes: 500_000_000,
    observed_at: new Date().toISOString(),
    pressure_observed_at: stalePressure,
    pressure_age_seconds: 120,
    write_pressure: true,
    failure_storage: {
      reason: "disk_quota_exceeded",
      location: "executor_runtime",
      executor_label: "sprites",
      observed_at: stalePressure,
    },
    revision: 2,
  });
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("Last reported");
  await expect(notice).toContainText("Delivery is blocked");
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Pressure observed",
  );
  await expect(testPage.getByTestId("session-delivery-storage-details-panel")).toContainText(
    "Capacity observed",
  );
  await expect(editor).toHaveText("Phone draft survives guidance updates");
  await prCapture.screenshot(`delivery-storage-pressure-stale-${testInfo.project.name}`, {
    caption: "Phone keeps old pressure last reported while capacity is fresh.",
  });

  await notify!({
    ...base,
    state: "warning",
    available_bytes: 100_000_000,
    pressure_observed_at: new Date().toISOString(),
    pressure_age_seconds: 0,
    revision: 3,
  });
  await expect(notice).toHaveCount(1);
  await expect(notice).toContainText("sprites: Disk 90% used");
  await expect(notice).not.toContainText("Delivery is blocked");
  await expect(editor).toHaveText("Phone draft survives guidance updates");

  await notify!({ ...base, state: "normal", available_bytes: 500_000_000, revision: 4 });
  await expect(notice).toHaveCount(0);
  await testPage.reload();
  await expect(testPage.getByTestId("chat-input-area")).toBeVisible();
  await expect(notice).toHaveCount(0);
});
