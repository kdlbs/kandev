import { expect } from "@playwright/test";
import path from "node:path";
import { backendFixture as test } from "../../fixtures/backend";
import { login, setupAdmin } from "../../helpers/auth";
import { loadInterimSettingsInterlockToken } from "../../helpers/interim-settings-interlock";

let reorderAgentId: string;

const ADMIN = {
  email: "profile-order-admin@e2e.dev",
  password: "adminpass123",
  displayName: "Order Admin",
};
const MEMBER = {
  email: "profile-order-member@e2e.dev",
  password: "memberpass123",
  displayName: "Order Member",
};

test.describe.serial("agent profile ordering member access", () => {
  test.beforeAll(async ({ backend, browser }) => {
    await backend.restart({
      KANDEV_FEATURES_AUTH: "true",
      KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "kandev-auth-profile-order.db"),
    });
    const admin = await browser.newContext({ baseURL: backend.frontendUrl });
    await setupAdmin(admin, backend.baseUrl, ADMIN);
    const interimSettingsInterlockToken = await loadInterimSettingsInterlockToken(backend.baseUrl);
    await login(admin, backend.baseUrl, ADMIN);
    const adminsAgentsResponse = await admin.request.get(`${backend.baseUrl}/api/v1/agents`);
    expect(adminsAgentsResponse.ok()).toBe(true);
    const { agents } = (await adminsAgentsResponse.json()) as {
      agents: Array<{ id: string; profiles: Array<{ id: string }> }>;
    };
    const agent = agents[0];
    if (!agent) throw new Error("The auth E2E backend must register an agent");
    reorderAgentId = agent.id;
    const created = await admin.request.post(`${backend.baseUrl}/api/v1/users`, {
      headers: { "X-Kandev-Interim-Settings-Interlock": interimSettingsInterlockToken },
      data: {
        email: MEMBER.email,
        password: MEMBER.password,
        display_name: MEMBER.displayName,
        role: "member",
      },
    });
    expect(created.status(), await created.text()).toBe(201);
  });

  test.afterAll(async ({ backend }) => {
    await backend.restart();
  });

  test("members cannot see reorder controls or save an order", async ({ browser, backend }) => {
    const context = await browser.newContext({ baseURL: backend.frontendUrl });
    await login(context, backend.baseUrl, MEMBER);
    const response = await context.request.get(`${backend.baseUrl}/api/v1/agents`);
    expect(response.ok()).toBe(true);
    const { agents } = (await response.json()) as {
      agents: Array<{ id: string; profiles: Array<{ id: string }> }>;
    };
    const agent = agents.find((item) => item.id === reorderAgentId);
    expect(agent).toBeDefined();
    const denied = await context.request.put(
      `${backend.baseUrl}/api/v1/agents/${reorderAgentId}/profiles/order`,
      {
        data: { profile_ids: agent!.profiles.map((profile) => profile.id) },
      },
    );
    expect(denied.status()).toBe(403);
    const page = await context.newPage();
    await page.goto("/settings/agents");
    await expect(page.getByTestId("agent-profile-drag-handle")).toHaveCount(0);
    await context.close();
  });
});
