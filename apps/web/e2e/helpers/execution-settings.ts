import { expect, type Page } from "@playwright/test";
import type { AppState } from "../../lib/state/store";
import type { SandboxBinding, SandboxPolicy, SandboxRevision } from "../../lib/types/sandbox";

type ExecutorContext = { id: string; name: string; key: string };

export async function installSandboxSettingsFixture(
  page: Page,
  profileId: string,
  executors: { id: string; name: string; type: string }[],
  options: {
    scope?: "agent" | "workflow_step" | "task";
    agentProfileId?: string;
    workspaceId?: string;
    workflowStepId?: string;
    workspacePolicies?: boolean;
  } = {},
) {
  const scope = options.scope ?? "agent";
  const agentProfileId = options.agentProfileId ?? profileId;
  const contexts = ["local", "worktree"].map((type, index) => {
    const executor = executors.find((item) => item.type === type);
    if (!executor) throw new Error(`Missing fixture executor: ${type}`);
    return { id: executor.id, name: executor.name, key: ["alpha", "beta"][index] };
  });
  const policies: SandboxRevision[] = ["alpha", "beta"].map((key) => ({
    id: `revision-${key}`,
    number: 1,
    schema_version: 1,
    digest: key.repeat(64).slice(0, 64),
    operator_id: "operator",
    org_id: "org",
    created_at: "2026-10-24T00:00:00Z",
    revoked: false,
    policy: {
      id: key,
      name: `Policy ${key}`,
      workspace_id: "",
      installation_id: key,
      provider_key: key,
      grants: {
        filesystem: [{ reference: "work_dir", writable: key === "alpha" }],
        network: [],
        kandev_mcp: true,
      },
      sources: [],
    },
  }));
  const workspacePolicy: SandboxRevision = {
    ...policies[0],
    id: "revision-workspace",
    policy: {
      ...policies[0].policy,
      id: "workspace-policy",
      name: "Workspace policy",
      workspace_id: options.workspaceId ?? "",
    },
  };
  let binding: SandboxBinding = {
    scope,
    target_id: profileId,
    workspace_id: options.workspaceId ?? "",
    version: 0,
    revision_id: "",
  };
  const writes: SandboxBinding[] = [];
  const executorWrites: SandboxBinding[] = [];
  const policyWrites: SandboxPolicy[] = [];
  let executorBinding: SandboxBinding = {
    scope: "executor",
    target_id: contexts[0].id,
    workspace_id: "",
    version: 0,
    revision_id: "",
    executor: {
      mode: "disabled",
      installation_id: "",
      provider_key: "",
      ceiling_revision_id: "",
      default_revision_id: "",
      eligible_revision_ids: [],
    },
  };
  let availabilityChecks = 0;
  function preview(request: import("@playwright/test").Request) {
    // Settings reads must never trigger an availability probe.
    if (!binding.revision_id) expect(availabilityChecks).toBe(0);
    const context = request.postDataJSON();
    expect(context.agent_profile_id).toBe(agentProfileId);
    if (scope === "workflow_step") expect(context.workflow_step_id).toBe(profileId);
    if (scope === "task") {
      expect(context.task_id).toBe(profileId);
      expect(context.workflow_step_id).toBe(options.workflowStepId);
    }
    const selected =
      policies.find(
        (item) =>
          item.policy.provider_key ===
          contexts.find((entry) => entry.id === context.executor_id)?.key,
      ) ?? policies[0];
    const invalid = Boolean(binding.revision_id && binding.revision_id !== selected.id);
    return {
      eligible: [selected],
      resolution_error: invalid ? "sandbox_policy_ineligible" : undefined,
      resolved: invalid
        ? null
        : {
            bindings: [],
            revision: selected,
            ceiling: selected,
            selection_source: binding.revision_id ? scope : "executor",
            executor_id: context.executor_id,
            binding_version: 1,
          },
    };
  }
  await page.route("**/api/v1/sandbox/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    let body: unknown;
    if (path.endsWith(`/bindings/executor/${contexts[0].id}`)) {
      if (request.method() === "PUT") {
        const payload = request.postDataJSON();
        expect(payload.expected_version).toBe(executorBinding.version);
        executorBinding = { ...payload.binding, version: executorBinding.version + 1 };
        executorWrites.push(executorBinding);
      }
      body = executorBinding;
    } else if (path.endsWith("/policies")) {
      if (request.method() === "POST") {
        const payload = request.postDataJSON();
        expect(payload.expected_revision).toBe(0);
        policyWrites.push(payload.policy);
        const created = {
          ...policies[0],
          id: "revision-created",
          digest: "c".repeat(64),
          policy: payload.policy,
        };
        policies.push(created);
        body = created;
      } else
        body = {
          policies:
            options.workspacePolicies &&
            new URL(request.url()).searchParams.get("workspace_id") === options.workspaceId
              ? [...policies, workspacePolicy]
              : policies,
        };
    } else if (path.endsWith(`/bindings/${scope}/${profileId}`)) {
      if (request.method() === "PUT") {
        const payload = request.postDataJSON();
        expect(payload.expected_version).toBe(binding.version);
        binding = { ...payload.binding, version: binding.version + 1 };
        writes.push(binding);
      }
      body = binding;
    } else if (path.endsWith("/providers")) {
      body = {
        providers: policies.map((item) => ({
          installation_id: item.policy.installation_id,
          provider_key: item.policy.provider_key,
          display_name: `Provider ${item.policy.provider_key}`,
          version: "1.0.0",
          available: true,
        })),
      };
    } else if (path.endsWith("/availability")) {
      availabilityChecks++;
      const context = request.postDataJSON();
      expect(context.agent_profile_id).toBe(agentProfileId);
      expect(context.executor_id).toBe(contexts[0].id);
      body = { available: true };
    } else if (path.endsWith("/preview")) {
      body = preview(request);
    } else if (path.includes("/revisions/")) {
      body = [...policies, workspacePolicy].find((item) => path.endsWith(item.id));
    } else {
      throw new Error(`Unexpected sandbox fixture request: ${request.method()} ${path}`);
    }
    await route.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(body),
    });
  });
  return { writes, executorWrites, policyWrites, contexts };
}

export async function enableSandboxSettingsFixture(page: Page) {
  await page.evaluate(() => {
    const store = (window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } })
      .__KANDEV_E2E_STORE__;
    const state = store.getState();
    state.setAuthState({
      mode: "enabled",
      authenticated: true,
      user: {
        id: "operator",
        email: "operator@example.test",
        display_name: "Operator",
        role: "admin",
        status: "active",
      },
    });
    state.setFeatures({ ...state.features, sandboxPolicies: true });
  });
}

export async function chooseSandboxSetting(
  page: Page,
  label: string,
  option: string,
  mobile: boolean,
) {
  await page.getByLabel(label, { exact: true }).click();
  if (mobile) await expect(page.getByRole("dialog")).toBeVisible();
  await page.getByRole("option", { name: option, exact: true }).click();
}

const choose = chooseSandboxSetting;

export async function checkSandboxSettingsSelection(
  page: Page,
  writes: SandboxBinding[],
  mobile: boolean,
  contexts: ExecutorContext[],
  scope: "agent" | "workflow_step" = "agent",
) {
  const section = page.getByTestId(`${scope}-sandbox-settings`);
  await expect(section).toBeVisible();
  await expect(section.getByLabel("Default policy", { exact: true })).toBeDisabled();
  await choose(page, "Executor context", contexts[0].name, mobile);
  await expect(section.getByText("Provider alpha (1.0.0)", { exact: false })).toBeVisible();
  await choose(page, "Default policy", "Policy alpha (1)", mobile);
  await expect(section.getByTestId("sandbox-permissions")).toContainText("{{kandev.work_dir}}");
  await page
    .getByRole("button", { name: /^Save( changes)?$/i })
    .first()
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].revision_id).toBe("revision-alpha");
  await expect(section).toContainText(
    `Saved selection source: ${scope === "agent" ? "Agent profile" : "Workflow step"}`,
  );
  await section.getByRole("button", { name: "Check availability", exact: true }).click();
  await expect(section.getByRole("status")).toHaveText(
    "Provider and runtime checks passed. Confinement is verified separately at launch.",
  );
  await choose(page, "Executor context", contexts[1].name, mobile);
  await expect(section.getByRole("status")).toHaveCount(0);
  await expect(section.getByRole("alert")).toContainText(
    "The saved policy is not eligible for this executor.",
  );
  await expect(section.getByLabel("Default policy", { exact: true })).toContainText("Policy alpha");
  expect(writes).toHaveLength(1);
  await choose(page, "Default policy", "Policy beta (1)", mobile);
  await expect(section.getByText("Provider beta (1.0.0)", { exact: false })).toBeVisible();
  await page
    .getByRole("button", { name: /^Save( changes)?$/i })
    .first()
    .click();
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1].revision_id).toBe("revision-beta");
  if (mobile) {
    const target = section.getByLabel("Executor context", { exact: true });
    expect((await target.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= document.documentElement.clientWidth,
      ),
    ).toBe(true);
  }
}

export async function checkExecutorSandboxSettings(
  page: Page,
  writes: SandboxBinding[],
  mobile: boolean,
  policies: SandboxPolicy[],
) {
  const section = page.getByTestId("executor-sandbox-settings");
  await expect(section).toBeVisible();
  await chooseSandboxSetting(page, "Sandbox mode", "Required", mobile);
  await chooseSandboxSetting(page, "Provider", "Provider alpha (1.0.0)", mobile);
  await createSandboxPolicyInSettings(page, mobile);
  expect(writes).toHaveLength(0);
  expect(policies).toHaveLength(1);
  expect(policies[0].grants).toEqual({
    filesystem: [
      { reference: "work_dir", writable: true },
      { reference: "agent_state", writable: true },
    ],
    network: [{ host: "api.example.test", port: 443, protocol: "tcp" }],
    environment_keys: ["API_KEY"],
    kandev_mcp: true,
  });
  await chooseSandboxSetting(page, "Permission ceiling", "New policy (1)", mobile);
  await section.getByRole("checkbox", { name: "New policy (1)", exact: true }).check();
  await chooseSandboxSetting(page, "Default policy", "New policy (1)", mobile);
  await page
    .getByRole("button", { name: /^Save( changes)?$/i })
    .first()
    .click();
  await expect.poll(() => writes.length).toBe(1);
  expect(writes[0].executor).toEqual({
    mode: "required",
    installation_id: "alpha",
    provider_key: "alpha",
    ceiling_revision_id: "revision-created",
    default_revision_id: "revision-created",
    eligible_revision_ids: ["revision-created"],
  });
}

export async function checkExecutorWorkspacePolicies(
  page: Page,
  writes: SandboxBinding[],
  mobile: boolean,
) {
  const section = page.getByTestId("executor-sandbox-settings");
  await chooseSandboxSetting(page, "Policy workspace", "E2E Workspace", mobile);
  await section
    .getByRole("checkbox", { name: "Workspace policy (1) E2E Workspace", exact: true })
    .check();
  for (const label of ["Permission ceiling", "Default policy"]) {
    await section.getByLabel(label, { exact: true }).click();
    await expect(
      page.getByRole("option", { name: "Workspace policy (1)", exact: true }),
    ).toHaveCount(0);
    await page.getByRole("option", { name: "New policy (1)", exact: true }).click();
  }
  await page
    .getByRole("button", { name: /^Save( changes)?$/i })
    .first()
    .click();
  await expect.poll(() => writes.length).toBe(2);
  expect(writes[1].executor?.eligible_revision_ids).toEqual([
    "revision-created",
    "revision-workspace",
  ]);
  expect(writes[1].executor?.default_revision_id).toBe("revision-created");
  expect(writes[1].executor?.ceiling_revision_id).toBe("revision-created");
  expect(writes[1].workspace_id).toBe("");
  if (mobile) {
    for (const width of [393, 700, 767]) {
      await page.setViewportSize({ width, height: 851 });
      await expect(section.getByLabel("Policy workspace", { exact: true })).toBeVisible();
      expect(
        (await section.getByLabel("Policy workspace", { exact: true }).boundingBox())!.height,
      ).toBeGreaterThanOrEqual(44);
      expect(
        await page.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      ).toBe(true);
    }
    await page.setViewportSize({ width: 393, height: 851 });
  }
}

async function createSandboxPolicyInSettings(page: Page, mobile: boolean) {
  await page.getByRole("button", { name: "Create policy", exact: true }).click();
  const dialog = page.getByRole("dialog");
  await dialog.getByLabel("Policy name", { exact: true }).fill("New policy");
  await dialog.getByLabel("Task work directory", { exact: true }).selectOption("write");
  await dialog
    .getByLabel("Agent configuration and session files", { exact: true })
    .selectOption("write");
  await dialog
    .getByRole("checkbox", { name: "Allow the task-scoped Kandev MCP connection", exact: true })
    .check();
  await dialog.getByRole("button", { name: "Add destination", exact: true }).click();
  await dialog.getByLabel("Destination host", { exact: true }).fill("api.example.test");
  await dialog.getByLabel("Port", { exact: true }).fill("443");
  await dialog
    .getByLabel("Permitted environment variable names, one per line. Do not enter values.", {
      exact: true,
    })
    .fill("API_KEY");
  const create = dialog.getByRole("button", { name: "Create policy", exact: true });
  if (mobile) {
    expect((await create.boundingBox())?.height).toBeGreaterThanOrEqual(44);
    expect(await dialog.evaluate((element) => element.scrollWidth <= element.clientWidth)).toBe(
      true,
    );
  }
  await create.click();
  await expect(dialog).toHaveCount(0);
}
