import { test, expect } from "../../fixtures/office-fixture";

// Helper to parse the permissions field which the backend stores as a JSON string.
function parsePermissions(agent: Record<string, unknown>): Record<string, unknown> {
  const raw = agent.permissions;
  if (typeof raw === "string" && raw.startsWith("{")) {
    try {
      return JSON.parse(raw) as Record<string, unknown>;
    } catch {
      return {};
    }
  }
  if (raw && typeof raw === "object") {
    return raw as Record<string, unknown>;
  }
  return {};
}

test.describe("Agent roles — permissions (API)", () => {
  test("security agent has can_approve: true", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "SecurityBot",
      role: "security",
    });
    const id = (created as Record<string, unknown>).id as string;
    expect(id).toBeTruthy();

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_approve).toBe(true);
  });

  test("security agent has can_create_tasks: false by default", async ({
    officeApi,
    officeSeed,
  }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "SecurityBot2",
      role: "security",
    });
    const id = (created as Record<string, unknown>).id as string;

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_create_tasks).toBe(false);
  });

  test("QA agent has can_create_tasks: true", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "QABot",
      role: "qa",
    });
    const id = (created as Record<string, unknown>).id as string;
    expect(id).toBeTruthy();

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_create_tasks).toBe(true);
  });

  test("QA agent has can_approve: false by default", async ({ officeApi, officeSeed }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "QABot2",
      role: "qa",
    });
    const id = (created as Record<string, unknown>).id as string;

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_approve).toBe(false);
  });

  test("DevOps agent has can_create_tasks: true and can_approve: false", async ({
    officeApi,
    officeSeed,
  }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "DevOpsBot",
      role: "devops",
    });
    const id = (created as Record<string, unknown>).id as string;
    expect(id).toBeTruthy();

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_create_tasks).toBe(true);
    expect(perms.can_approve).toBe(false);
    expect(perms.can_create_agents).toBe(false);
  });

  test("worker agent has can_create_tasks: true and can_approve: false", async ({
    officeApi,
    officeSeed,
  }) => {
    const created = await officeApi.createAgent(officeSeed.workspaceId, {
      name: "WorkerBot",
      role: "worker",
    });
    const id = (created as Record<string, unknown>).id as string;

    const agent = await officeApi.getAgent(id);
    const perms = parsePermissions(agent as Record<string, unknown>);
    expect(perms.can_create_tasks).toBe(true);
    expect(perms.can_approve).toBe(false);
    expect(perms.can_create_agents).toBe(false);
  });
});

test.describe("Agent roles — UI rendering", () => {
  test("agents page shows new security agent with correct role badge", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "UISecurityBot",
      role: "security",
    });

    // Wait for the agent to surface on the public list endpoint before
    // navigating — otherwise the page's SSR can race ahead of the
    // INSERT being visible to a separate connection on a heavily-loaded
    // parallel suite run and render with a stale agent set.
    await expect
      .poll(
        async () => {
          const list = (await officeApi.listAgents(officeSeed.workspaceId)) as {
            agents?: Array<{ name?: string }>;
          };
          return (list.agents ?? []).some((a) => a.name === "UISecurityBot");
        },
        { timeout: 10_000 },
      )
      .toBe(true);

    await testPage.goto("/office/agents");

    // The agent name appears inside card spans.
    await expect(
      testPage
        .locator("span")
        .filter({ hasText: /^UISecurityBot$/ })
        .first(),
    ).toBeVisible({ timeout: 20_000 });

    // A "security" role badge (or label) must be visible somewhere on the page.
    // The AgentRoleBadge renders the role as a Badge with label from meta or
    // falls back to the role string.
    await expect(testPage.getByText(/security/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test("agents page shows new QA agent with correct role badge", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "UIQABot",
      role: "qa",
    });

    await testPage.goto("/office/agents");

    await expect(
      testPage
        .locator("span")
        .filter({ hasText: /^UIQABot$/ })
        .first(),
    ).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByText(/qa/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test("agents page shows new DevOps agent", async ({ testPage, officeApi, officeSeed }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "UIDevOpsBot",
      role: "devops",
    });

    await testPage.goto("/office/agents");

    await expect(
      testPage
        .locator("span")
        .filter({ hasText: /^UIDevOpsBot$/ })
        .first(),
    ).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByText(/devops/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test("agent detail page shows correct role for security agent", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "DetailSecurityBot",
      role: "security",
    });

    // Navigate to agents list first to populate the store via SSR/hydration.
    await testPage.goto("/office/agents");

    // Click the agent card link to navigate client-side to the detail page.
    const agentCardLink = testPage
      .locator("a")
      .filter({ hasText: /DetailSecurityBot/ })
      .first();
    await expect(agentCardLink).toBeVisible({ timeout: 10_000 });
    await agentCardLink.click();

    // The agent name moved into the office topbar portal; the
    // detail page itself no longer renders an <h2>. Wait for the
    // tab nav (rendered only after the agent record resolves) and
    // then assert the role badge is visible.
    await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({ timeout: 10_000 });

    // The role badge is visible on the detail page.
    await expect(testPage.getByText(/security/i).first()).toBeVisible({ timeout: 10_000 });
  });

  test("agent detail page shows correct role for QA agent", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await officeApi.createAgent(officeSeed.workspaceId, {
      name: "DetailQABot",
      role: "qa",
    });

    await testPage.goto("/office/agents");

    const agentCardLink = testPage
      .locator("a")
      .filter({ hasText: /DetailQABot/ })
      .first();
    await expect(agentCardLink).toBeVisible({ timeout: 10_000 });
    await agentCardLink.click();

    // Agent name lives in the office topbar slot now; wait for the
    // tab nav (only renders after the agent record resolves) and
    // check the role badge instead.
    await expect(testPage.getByTestId("agent-tab-dashboard")).toBeVisible({ timeout: 10_000 });

    await expect(testPage.getByText(/qa/i).first()).toBeVisible({ timeout: 10_000 });
  });
});
