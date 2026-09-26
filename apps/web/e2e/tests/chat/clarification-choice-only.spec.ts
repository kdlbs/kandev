import { expect, test } from "../../fixtures/test-base";
import { seedClarificationSession } from "../../helpers/clarification";
import { waitForFiniteAnimations } from "../../helpers/pr-capture";

test("requires an offered choice when custom text is disabled", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(60_000);
  const session = await seedClarificationSession(
    testPage,
    apiClient,
    seedData,
    "Clarification Choice Only",
    { scenario: "clarification-no-other" },
  );

  const overlay = session.clarificationOverlay();
  await expect(overlay).toBeVisible({ timeout: 30_000 });
  await expect(session.clarificationCustomInput()).toHaveCount(0);
  if (prCapture.capturing) await waitForFiniteAnimations(overlay);
  await prCapture.screenshot("clarification-choice-only-desktop", {
    caption: "Desktop clarification offers only the choices allowed by Codex",
  });
  await session.clarificationOption("Fast").click();

  await expect(session.idleInput()).toBeVisible({ timeout: 30_000 });
  await expect(session.chat).toContainText("You answered");
});
