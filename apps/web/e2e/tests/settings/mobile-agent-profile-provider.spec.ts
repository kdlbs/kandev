import { test, expect } from "../../fixtures/test-base";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";

/**
 * Mobile layout of the OpenAI-compatible provider section: the fields stack in
 * one column and entering a base URL does not push the document into
 * horizontal overflow.
 */
test.describe("Agent profile provider section on mobile", () => {
  test("provider fields are reachable in one column without overflow", async ({
    testPage,
    apiClient,
  }) => {
    test.setTimeout(120_000);

    const { agents } = await apiClient.listAgents();
    const agent = agents.find((a) => a.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "Mobile provider test", {
      model: "mock-fast",
    });

    try {
      await testPage.goto(`/settings/agents/${agent.name}/profiles/${profile.id}`);

      const kindSelect = testPage.getByTestId("provider-kind-select");
      await expect(kindSelect).toBeVisible({ timeout: 15_000 });
      await kindSelect.click();
      await testPage.getByRole("option", { name: /OpenAI-compatible provider/i }).click();

      const baseUrl = testPage.getByTestId("provider-base-url-input");
      await expect(baseUrl).toBeVisible();
      await baseUrl.fill("http://localhost:20128/v1");

      await assertNoDocumentHorizontalOverflow(testPage, "provider section (mobile)");
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });
});
