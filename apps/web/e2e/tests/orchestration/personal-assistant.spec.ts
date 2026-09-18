import { test, expect } from "../../fixtures/test-base";
import { ASSISTANT_ENV, exerciseExampleAssistant } from "../../helpers/personal-assistant";
test("assistant resumes privately and resolves native input with Office disabled", async ({
  testPage: page,
  backend,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180000);
  await backend.restart({ ...ASSISTANT_ENV, KANDEV_FEATURES_PERSONAL_ASSISTANT: "false" });
  const disabledReads: string[] = [];
  const watch = (request: { url: () => string }) => {
    if (request.url().includes("/api/v1/orchestration/assistant"))
      disabledReads.push(request.url());
  };
  page.on("request", watch);
  await page.goto("/assistant");
  await expect(page.getByText("Enable Personal assistant", { exact: false })).toBeVisible();
  expect(disabledReads).toEqual([]);
  page.off("request", watch);
  await backend.restart(ASSISTANT_ENV);
  await page.setViewportSize({ width: 1440, height: 1000 });
  await exerciseExampleAssistant(page, backend, apiClient, seedData, false);
});
