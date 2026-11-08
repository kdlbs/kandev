import { expect, test } from "../../fixtures/test-base";
import {
  assertExplicitLaunchACPTrace,
  assertExplicitLaunchMessage,
  createStartCreatedPromptFixture,
  expectExplicitLaunchVisible,
  expectNoNativePromptBeforeLaunch,
  launchExplicitBrief,
  waitForExplicitLaunchACPTrace,
  waitForExplicitLaunchMessage,
} from "../../helpers/start-created-prompt-integrity";
import { SessionPage } from "../../pages/session-page";

test.setTimeout(240_000);

const BRIEF =
  "Independent READ-ONLY review. Do not edit, rebase, commit, push, or publish. Report only blocking findings.";
const REPLACING_STEP = "Read the retained implementation plan and continue with the review.";

test("mobile: explicit launch brief remains reachable once after reload", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const fixture = await createStartCreatedPromptFixture(backend, apiClient, seedData, {
    stepPrompt: REPLACING_STEP,
  });
  try {
    expectNoNativePromptBeforeLaunch(fixture);
    await launchExplicitBrief(apiClient, fixture, BRIEF);

    const message = await waitForExplicitLaunchMessage(apiClient, fixture);
    assertExplicitLaunchMessage(message, BRIEF, REPLACING_STEP);
    await waitForExplicitLaunchACPTrace(fixture);
    assertExplicitLaunchACPTrace(fixture, message, BRIEF, REPLACING_STEP);

    const session = new SessionPage(testPage);
    await testPage.goto(`/t/${fixture.taskId}`);
    await session.waitForLoad();
    await expectExplicitLaunchVisible(session, BRIEF);

    await testPage.reload();
    await session.waitForLoad();
    await expectExplicitLaunchVisible(session, BRIEF);
    expect(
      await testPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth + 1),
    ).toBe(true);
  } finally {
    await fixture.dispose();
  }
});
