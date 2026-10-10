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

test("desktop navigates from an active task to and between archived conversations without launching sessions", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(180_000);
  const archived = await seedArchivedConversations(
    apiClient,
    seedData,
    `Archived SPA ${Date.now()}`,
  );
  const active = await seedActiveConversation(
    apiClient,
    seedData,
    `Active before archived SPA ${Date.now()}`,
    "active conversation before archive navigation",
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
  await selectArchivedSidebarView(testPage);

  const rowA = session.sidebar
    .getByTestId("sidebar-task-item")
    .filter({ hasText: archived[0].title });
  await expect(rowA).toBeVisible({ timeout: 20_000 });
  const documentRequestCount = documentRequests.length;
  await rowA.click();
  await expect(testPage).toHaveURL(
    new RegExp(`/t/${archived[0].id}\\?sessionId=${archived[0].sessionId}$`),
  );
  await expect(session.activeChat().getByText(archived[0].response).last()).toBeVisible({
    timeout: 30_000,
  });
  await expect(testPage.getByTestId("task-unarchive-button")).toBeVisible();

  const rowB = session.sidebar
    .getByTestId("sidebar-task-item")
    .filter({ hasText: archived[1].title });
  await expect(rowB).toBeVisible({ timeout: 20_000 });
  await rowB.click();
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
