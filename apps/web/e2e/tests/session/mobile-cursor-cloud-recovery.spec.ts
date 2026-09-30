import { expect, test } from "../../fixtures/cursor-cloud";
import { createCursorCloudTask, disableCursorCloudExecutors } from "./cursor-cloud-helpers";

test.afterEach(async ({ apiClient }) => {
  await disableCursorCloudExecutors(apiClient);
});

test("resolves an uncertain submission from the phone recovery sheet", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone recovery",
  );
  await expect.poll(() => cursorCloud.prompts.length).toBe(1);
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByText("Remote result is ready")).toBeVisible();

  cursorCloud.failNextSubmission("unknown-followup");
  const composer = testPage.getByTestId("chat-input-editor");
  await expect(composer).toHaveAttribute("contenteditable", "true");
  await composer.fill("Phone follow-up with uncertain response");
  const submit = testPage.getByTestId("submit-message-button");
  const submitBox = await submit.boundingBox();
  expect(submitBox).not.toBeNull();
  expect(submitBox!.width).toBeGreaterThanOrEqual(44);
  expect(submitBox!.height).toBeGreaterThanOrEqual(44);
  await expect(submit).toBeInViewport();
  await submit.tap();
  await expect.poll(() => cursorCloud.prompts.length).toBe(2);
  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeVisible();

  const resolutionResponse = await testPage.request.get(
    `${backend.baseUrl}/api/v1/tasks/${task.id}/sessions/${task.session_id}/cursor-cloud/submission`,
  );
  expect(resolutionResponse.ok()).toBe(true);
  expect(await resolutionResponse.json()).toMatchObject({
    state: "unknown",
    candidates: [{ runId: "run-2" }],
  });

  await testPage.getByRole("button", { name: "Resolve submission" }).click();
  const candidate = testPage.getByRole("radio", { name: /run-2/ });
  await expect(candidate).toBeVisible({ timeout: 15_000 });
  await candidate.check();
  await testPage.getByRole("button", { name: "Bind selected run" }).click();

  await expect(testPage.getByTestId("cursor-cloud-submission-unknown")).toBeHidden();
  await expect(testPage.getByText("Follow-up run-2 result is ready")).toBeVisible();
  expect(cursorCloud.prompts).toHaveLength(2);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");
  expect(cursorCloud.prompts[1]).toContain("Phone follow-up with uncertain response");
});

test("keeps phone cancellation pending until Cursor confirms termination", async ({
  backend,
  cursorCloud,
  seedData,
  apiClient,
  testPage,
}) => {
  cursorCloud.reset();
  cursorCloud.holdNextStreamOpen();
  cursorCloud.holdCancellationPending();
  const { task } = await createCursorCloudTask(
    apiClient,
    backend,
    seedData,
    "Cursor Cloud phone cancellation",
  );
  await expect.poll(() => cursorCloud.streamRequestCount()).toBeGreaterThan(0);
  await testPage.goto(`/tasks/${task.id}`);
  await expect(testPage.getByTestId("cursor-cloud-task-surface")).toBeVisible();

  const stop = testPage.getByRole("button", { name: "Stop" });
  await expect(stop).toBeEnabled();
  await stop.click();
  const stopping = testPage.getByRole("button", { name: "Stopping remote work..." });
  await expect(stopping).toBeVisible();
  await expect
    .poll(
      () =>
        cursorCloud.requests.filter(
          (request) => request.method === "POST" && request.path.endsWith("/cancel"),
        ).length,
    )
    .toBe(1);

  cursorCloud.releaseCancellation();
  await expect(testPage.getByRole("button", { name: "Stop" })).toBeDisabled();
  expect(cursorCloud.prompts).toHaveLength(1);
  expect(cursorCloud.prompts[0]).toContain("Implement the requested remote change");
});
