import { test, expect } from "../../fixtures/test-base";
import {
  installIdentityFixture,
  expectIdentity,
  expectExpandedDiagnostics,
} from "./agent-runtime-identity-helpers";

// @covers AC-AGENTS-RUNTIME-IDENTITY-001.1, AC-AGENTS-RUNTIME-IDENTITY-001.2, AC-AGENTS-RUNTIME-IDENTITY-001.3, AC-AGENTS-RUNTIME-IDENTITY-001.5, AC-AGENTS-RUNTIME-IDENTITY-001.6
for (const native of [false, true]) {
  test(`shows ${native ? "native" : "managed"} runtime with touch-accessible diagnostics`, async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installIdentityFixture(testPage, native);
    await testPage.goto("/settings/agents");
    const { card, diagnostics } = await expectIdentity(testPage, native);
    const identityBox = await card.getByTestId("session-runtime-copilot-acp").boundingBox();
    const cardBox = await card.boundingBox();
    expect(identityBox!.width).toBeGreaterThan(cardBox!.width * 0.75);
    const trigger = diagnostics.locator("summary");
    expect((await trigger.boundingBox())!.height).toBeGreaterThanOrEqual(44);
    await prCapture.screenshot(`mobile-runtime-${native ? "native" : "managed"}`, {
      caption: "Phone session runtime with diagnostics closed",
    });
    await trigger.tap();
    await expectExpandedDiagnostics(testPage);
    await prCapture.screenshot(`mobile-diagnostics-${native ? "native" : "managed"}`, {
      caption: "Phone diagnostics with a long local path",
    });
    expect(runtime.postCount()).toBe(0);
    expect(runtime.previewCount()).toBe(0);
  });
}
