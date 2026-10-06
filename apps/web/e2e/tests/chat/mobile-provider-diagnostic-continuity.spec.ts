// Filename routes this chat regression to the configured mobile-chrome project.
import { test } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import {
  assertProviderDiagnosticRendering,
  assertProviderDiagnosticStorage,
  openProviderDiagnosticTask,
  seedProviderDiagnosticTask,
  waitForProviderDiagnosticTurn,
} from "./provider-diagnostic-continuity-helpers";

test("mobile provider diagnostic prose stays in one assistant message through reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);

  const { task, sessionId } = await seedProviderDiagnosticTask(
    apiClient,
    seedData,
    "Provider diagnostic continuity mobile",
  );
  const session = await openProviderDiagnosticTask(testPage, task.id);
  await waitForProviderDiagnosticTurn(apiClient, task.id, sessionId, session);

  const liveMessageId = await assertProviderDiagnosticStorage(apiClient, sessionId);
  await assertProviderDiagnosticRendering(session, liveMessageId);
  await assertNoDocumentHorizontalOverflow(
    testPage,
    "provider diagnostic continuity before reload",
  );

  await testPage.reload();
  await session.waitForLoad();
  await session.waitForChatIdle({ timeout: 30_000 });
  const reloadedMessageId = await assertProviderDiagnosticStorage(apiClient, sessionId);
  await assertProviderDiagnosticRendering(session, reloadedMessageId);
  if (reloadedMessageId !== liveMessageId) {
    throw new Error(`Reload changed persisted provider diagnostic message ID ${liveMessageId}`);
  }
  await assertNoDocumentHorizontalOverflow(testPage, "provider diagnostic continuity after reload");
});
