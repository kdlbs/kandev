import { execFileSync } from "node:child_process";
import { test, expect } from "../../fixtures/test-base";
import { ASSISTANT_ENV, selectExampleAssistant } from "../../helpers/personal-assistant";

// @covers AC-ORCHESTRATION-ASSISTANT-002.3, AC-ORCHESTRATION-ASSISTANT-003.1
test("assistant pages safe capabilities and shows the account's specific unblock action", async ({
  testPage: page,
  backend,
  apiClient: api,
  seedData: seed,
}) => {
  test.setTimeout(180000);
  await backend.restart(ASSISTANT_ENV);
  const binding = await selectExampleAssistant(page, backend, api, seed);
  const { agents } = await api.listAgents();
  const provider = agents.find((row) => row.profiles?.some((p) => p.id === seed.agentProfileId));
  expect(provider).toBeTruthy();
  for (let index = 0; index < 51; index++) {
    await api.createAgentProfile(
      provider!.id,
      `Example catalog ${String(index).padStart(2, "0")}`,
      {
        model: "mock-fast",
        env_vars: [
          { key: "EXAMPLE_PRIVATE_CONFIG", value: "SYNTHETIC_PRIVATE_CONFIGURATION_CANARY" },
        ],
      },
    );
  }
  const base = `${backend.baseUrl}/api/v1/orchestration/assistant`;
  const descriptor = await page.request.put(`${base}/credentials/example-vault`, {
    data: {
      expected_revision: 0,
      resolver: "bitwarden",
      reference: "example-item",
      purpose: "Example documentation account",
      profile_id: seed.agentProfileId,
      account: "example-account",
      environment: "example",
      scope: "workspace",
      fields: ["username"],
      unlock_policy: "Ask the owner to connect the example vault.",
    },
  });
  expect(descriptor.status()).toBe(200);
  const tasksBefore = await api.listTasks(seed.workspaceId);
  const repositoryBefore = execFileSync(
    "git",
    ["-C", seed.repositoryPath, "status", "--porcelain"],
    { encoding: "utf8" },
  );
  await page.reload();
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  await page
    .locator("summary")
    .filter({ hasText: /^Capabilities$/ })
    .click();
  const capabilities = page.getByRole("region", { name: "Capabilities", exact: true });
  await expect(capabilities.getByRole("listitem")).toHaveCount(50);
  await capabilities.getByRole("button", { name: "Load more", exact: true }).click();
  await expect.poll(() => capabilities.getByRole("listitem").count()).toBeGreaterThan(50);
  await expect(capabilities).toContainText("Example catalog 50");
  await expect(capabilities).not.toContainText("SYNTHETIC_PRIVATE_CONFIGURATION_CANARY");
  await page
    .locator("summary")
    .filter({ hasText: /^Account and credential health$/ })
    .click();
  const credentials = page.getByRole("region", {
    name: "Account and credential health",
    exact: true,
  });
  await expect(credentials).toContainText("Example documentation account");
  await expect(credentials).toContainText("unavailable");
  await expect(credentials).toContainText("Ask the owner to connect the example vault.");
  await page.reload();
  await page.getByRole("tab", { name: "Details", exact: true }).click();
  await page
    .locator("summary")
    .filter({ hasText: /^Account and credential health$/ })
    .click();
  await expect(credentials).toContainText("Ask the owner to connect the example vault.");
  expect(await api.listTasks(seed.workspaceId)).toEqual(tasksBefore);
  expect(
    execFileSync("git", ["-C", seed.repositoryPath, "status", "--porcelain"], { encoding: "utf8" }),
  ).toBe(repositoryBefore);
  expect((await api.listTaskSessions(binding.conversation_id)).sessions).toEqual([]);
});
