import { test } from "../../fixtures/test-base";
import { dwell } from "../../helpers/causal-waits";
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

test("desktop: explicit launch brief survives a replacing step template", async ({
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
    await dwell(
      1000,
      "negative-assertion",
      "reload must not dispatch the native launch prompt again",
    );
    assertExplicitLaunchACPTrace(fixture, message, BRIEF, REPLACING_STEP);
  } finally {
    await fixture.dispose();
  }
});

test("desktop: placeholder step template inserts the explicit launch brief once", async ({
  testPage,
  apiClient,
  backend,
  seedData,
}) => {
  const stepPrompt = `${REPLACING_STEP}\n\n{{task_prompt}}`;
  const fixture = await createStartCreatedPromptFixture(backend, apiClient, seedData, {
    stepPrompt,
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
  } finally {
    await fixture.dispose();
  }
});
