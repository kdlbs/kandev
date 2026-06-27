import { test } from "../../fixtures/test-base";
import {
  expectMermaidDiagramFitsViewport,
  seedTaskWithWideMermaidMessage,
} from "./mermaid-fit-helpers";

test.describe("Mobile Mermaid diagram fit", () => {
  test("auto-scales wide diagrams to the mobile chat viewport", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    test.setTimeout(90_000);

    const session = await seedTaskWithWideMermaidMessage(
      testPage,
      apiClient,
      seedData,
      "Mermaid Fit Mobile",
    );

    await expectMermaidDiagramFitsViewport(session);
  });
});
