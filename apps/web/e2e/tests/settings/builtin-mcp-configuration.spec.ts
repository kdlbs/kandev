import { expect, test } from "../../fixtures/test-base";

test("configuration chats bind only Core and Configuration and expose the real catalog", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  test.setTimeout(120_000);
  await apiClient.updateWorkspace(seedData.workspaceId, {
    default_config_agent_profile_id: seedData.agentProfileId,
  });
  await testPage.goto("/settings/agents");
  await testPage.getByRole("button", { name: "Configuration Chat", exact: true }).click();
  const chat = testPage.getByTestId("config-chat-popover");
  const setup = chat.getByTestId("config-chat-setup");
  await setup
    .getByPlaceholder("Ask anything about your configuration...")
    .fill("e2e:mcp:kandev-configuration:list_shared_prompts_kandev({})");
  const created = testPage.waitForResponse(
    (response) =>
      response.request().method() === "POST" &&
      new URL(response.url()).pathname.endsWith("/config-chat"),
  );
  await setup.getByRole("button", { name: "Start configuration chat" }).click();
  const response = await created;
  expect(response.ok()).toBeTruthy();
  const { session_id } = await response.json();
  await expect
    .poll(
      async () => {
        const result = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${session_id}/mcp-built-in`,
        );
        expect(result.status).toBe(200);
        const view = await result.json();
        return view.bound?.manifest.packages;
      },
      { timeout: 30_000 },
    )
    .toEqual(["configuration", "core"]);
  await expect
    .poll(
      async () => {
        const result = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${session_id}/mcp-built-in`,
        );
        const view = await result.json();
        return view.history?.current?.servers
          ?.find((server: { name: string }) => server.name === "kandev-configuration")
          ?.tools?.some((tool: { name: string }) => tool.name === "list_shared_prompts_kandev");
      },
      { timeout: 30_000 },
    )
    .toBe(true);
});
