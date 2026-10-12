import { expect, test } from "../../fixtures/test-base";
import {
  captureGatewayRequests,
  sessionLaunchRequests,
} from "../../helpers/archived-session-recovery";
import { SessionPage } from "../../pages/session-page";
import {
  seedActiveConversation,
  seedArchivedConversations,
  selectArchivedSidebarView,
} from "./archived-task-navigation-helpers";

test("phone navigates from an active task to and between archived conversations without launching sessions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const archived = await seedArchivedConversations(
    apiClient,
    seedData,
    `Mobile archived SPA ${Date.now()}`,
  );
  const active = await seedActiveConversation(
    apiClient,
    seedData,
    `Active before mobile archived SPA ${Date.now()}`,
    "active phone conversation before archive navigation",
  );
  const requests = captureGatewayRequests(testPage);
  const documentRequests: string[] = [];
  testPage.on("request", (request) => {
    if (request.isNavigationRequest() && request.frame() === testPage.mainFrame()) {
      documentRequests.push(request.url());
    }
  });

  await testPage.goto(`/t/${active.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.activeChat().getByText(active.response).last()).toBeVisible({
    timeout: 30_000,
  });

  const openArchivedPicker = async () => {
    await testPage.getByTestId("mobile-task-picker-trigger").tap();
    return testPage.getByRole("dialog", { name: "Tasks" });
  };
  let sheet = await openArchivedPicker();
  await selectArchivedSidebarView(testPage);
  const rowA = sheet.getByTestId("sidebar-task-item").filter({ hasText: archived[0].title });
  await expect(rowA).toBeVisible({ timeout: 20_000 });
  const documentRequestCount = documentRequests.length;
  await rowA.tap();
  await expect(sheet).toBeHidden();
  await expect(testPage).toHaveURL(
    new RegExp(`/t/${archived[0].id}\\?sessionId=${archived[0].sessionId}$`),
  );
  await expect(session.activeChat().getByText(archived[0].response).last()).toBeVisible({
    timeout: 30_000,
  });
  await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();

  sheet = await openArchivedPicker();
  const rowB = sheet.getByTestId("sidebar-task-item").filter({ hasText: archived[1].title });
  await expect(rowB).toBeVisible({ timeout: 20_000 });
  await rowB.tap();
  await expect(sheet).toBeHidden();
  await expect(testPage).toHaveURL(
    new RegExp(`/t/${archived[1].id}\\?sessionId=${archived[1].sessionId}$`),
  );
  await expect(session.activeChat().getByText(archived[1].response).last()).toBeVisible({
    timeout: 30_000,
  });
  expect(documentRequests).toHaveLength(documentRequestCount);

  for (const conversation of archived) {
    expect(sessionLaunchRequests(requests, conversation.sessionId)).toEqual([]);
  }
  await testPage.reload();
  await session.waitForLoad();
  await expect(session.activeChat().getByText(archived[1].response).last()).toBeVisible({
    timeout: 30_000,
  });
  for (const conversation of archived) {
    expect(sessionLaunchRequests(requests, conversation.sessionId)).toEqual([]);
  }
});
