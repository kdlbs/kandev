import { test, expect } from "../../fixtures/test-base";
import {
  installIdentityFixture,
  expectIdentity,
  expectExpandedDiagnostics,
} from "./agent-runtime-identity-helpers";

// @covers AC-AGENTS-RUNTIME-IDENTITY-001.1, AC-AGENTS-RUNTIME-IDENTITY-001.2, AC-AGENTS-RUNTIME-IDENTITY-001.3, AC-AGENTS-RUNTIME-IDENTITY-001.6
for (const native of [false, true]) {
  test(`shows ${native ? "native" : "managed"} runtime independently of local CLI diagnostics`, async ({
    testPage,
    prCapture,
  }) => {
    const runtime = await installIdentityFixture(testPage, native);
    await testPage.goto("/settings/agents");
    const { diagnostics } = await expectIdentity(testPage, native);
    await prCapture.screenshot(`desktop-runtime-${native ? "native" : "managed"}`, {
      caption: "Session runtime with local CLI diagnostics closed",
    });
    await diagnostics.locator("summary").focus();
    await testPage.keyboard.press("Enter");
    await expectExpandedDiagnostics(testPage);
    await prCapture.screenshot(`desktop-diagnostics-${native ? "native" : "managed"}`, {
      caption: "Expanded local CLI diagnostics",
    });
    expect(runtime.postCount()).toBe(0);
    expect(runtime.previewCount()).toBe(0);
  });
}
