import { test } from "../../fixtures/test-base";
import {
  expectMermaidMessageUnchanged,
  expectMermaidRecovery,
  expectMermaidDiagramFitsViewport,
  GLUED_MERMAID_MESSAGE,
  seedTaskWithWideMermaidMessage,
} from "./mermaid-fit-helpers";

test.describe("Mobile Mermaid diagram fit", () => {
  test("auto-scales wide diagrams to the mobile chat viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const { session } = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fit Mobile",
    );

    await expectMermaidDiagramFitsViewport(session);
  });

  // @covers AC-UI-MERMAID-RENDERING-002.4, AC-UI-MERMAID-RENDERING-002.5
  test("separates glued prose and preserves the saved message after reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const seeded = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fence Recovery Mobile",
      GLUED_MERMAID_MESSAGE,
    );

    await expectMermaidRecovery(seeded.session);
    await expectMermaidMessageUnchanged(apiClient, seeded.sessionId, GLUED_MERMAID_MESSAGE);

    await testPage.reload();
    await seeded.session.waitForLoad();
    await expectMermaidRecovery(seeded.session);
    await expectMermaidMessageUnchanged(apiClient, seeded.sessionId, GLUED_MERMAID_MESSAGE);
  });
});
