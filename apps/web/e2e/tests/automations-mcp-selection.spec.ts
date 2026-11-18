import { waitForFiniteAnimations } from "../helpers/pr-capture";
import { watchWs } from "../helpers/causal-waits";
import { test, expect } from "../fixtures/test-base";
import {
  OPTIONAL_AUTOMATION_PACKAGES,
  chooseAutomationPackage,
  verifyAutomationMCPDeleteWarning,
  saveAutomationMCP,
  verifyAutomationMCPRecovery,
  verifyAutomationMCPRepositoryInheritance,
} from "../helpers/automation-mcp";
import type { Automation, AutomationRun } from "@/lib/types/automation";
import type { BuiltinMCPSessionView } from "@/lib/types/builtin-mcp";

// @covers AC-OFFICE-AUTOMATION-MCP-001.2
test("matches MCP inheritance to the saved repository for a loaded automation", async ({
  testPage,
  apiClient,
  seedData,
  backend,
  prCapture,
}) => {
  await verifyAutomationMCPRepositoryInheritance({
    testPage,
    apiClient,
    seedData,
    backend,
    prCapture,
  });
});

test("saves all native packages and workspace servers, then invokes native tools in a private run", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  test.setTimeout(180000);
  const server = await apiClient.createMCPServer(seedData.workspaceId, {
    runtime_name: "automation-reports",
    display_name: "Automation reports",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://reports.example.test/mcp" },
    source: "custom",
  });
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Native MCP report",
    repositoryMode: "none",
    agentProfileId: seedData.agentProfileId,
    executorProfileId: seedData.worktreeExecutorProfileId,
    prompt: [
      "e2e:mcp:kandev-configuration:list_shared_prompts_kandev({})",
      'e2e:mcp:kandev-rich-output:show_rich_output_kandev({"version":2,"catalog_version":1,"title":"Automation report","spec":{"root":"note","elements":{"note":{"type":"Callout","props":{"kind":"info","text":"Automation report"},"children":[]}}}})',
      'e2e:mcp:kandev-walkthroughs:show_walkthrough_kandev({"title":"Automation package report","steps":[{"file":"notes.md","line":1,"text":"Scheduled report"}]})',
      'e2e:mcp:kandev-diagnostics:get_diagnostic_bundle_kandev({"source":"backend"})',
    ].join("\n"),
  });
  const watcher = watchWs(testPage);
  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/automations/${automation.id}`);
  const section = testPage.getByTestId("automation-mcp-access");
  await expect(
    section.getByTestId("automation-mcp-package-core").locator('[data-slot="badge"]'),
  ).toHaveText("Required");
  for (const id of OPTIONAL_AUTOMATION_PACKAGES) await chooseAutomationPackage(testPage, id, true);
  const serverRow = section.getByTestId(`automation-mcp-server-${server.id}`);
  await serverRow.click();
  await section.evaluate((element) => element.scrollIntoView({ block: "start" }));
  if (prCapture.capturing) await waitForFiniteAnimations(testPage.locator("body"));
  await prCapture.screenshot("automation-mcp-desktop", {
    caption: "Automation MCP selection on desktop",
  });
  await section.screenshot({ path: "../../artifacts/mcp-switches/list-automation-desktop.png" });
  await section.getByTestId("automation-mcp-package-configuration").click();
  const detail = section.getByTestId("automation-mcp-package-detail");
  expect(
    (await detail.getByTestId("automation-mcp-choice-configuration").boundingBox())!.height,
  ).toBeCloseTo(28, 0);
  await detail.evaluate((element) => element.scrollIntoView({ block: "start" }));
  if (prCapture.capturing) await waitForFiniteAnimations(testPage.locator("body"));
  await detail.screenshot({ path: "../../artifacts/mcp-switches/automation-desktop.png" });
  await prCapture.screenshot("automation-mcp-package-detail-desktop", {
    caption: "Automation package detail uses the shared MCP package presentation",
  });
  await detail.getByRole("button", { name: "Back to packages", exact: true }).click();
  await saveAutomationMCP(testPage, watcher);
  let saved = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  expect(saved.mcp_selection?.definition_ids).toEqual([server.id]);
  expect(saved.mcp_selection?.builtin_packages).toEqual(["core", ...OPTIONAL_AUTOMATION_PACKAGES]);
  await testPage.reload();
  await expect(serverRow.getByRole("checkbox")).toBeChecked();
  for (const id of OPTIONAL_AUTOMATION_PACKAGES)
    await expect(
      section.getByTestId(`automation-mcp-choice-${id}`).getByRole("switch"),
    ).toBeChecked();
  // The external endpoint is only a saved-definition fixture. Native execution uses no external service.
  await serverRow.click();
  await saveAutomationMCP(testPage, watcher);
  const fired = await apiClient.triggerAutomationManual(automation.id);
  expect(fired.skipped).not.toBe(true);
  expect(fired.run_task_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const result = await apiClient.wsRequest<{ title?: string }>("task.walkthrough.get", {
          task_id: fired.run_task_id,
        });
        return result?.title;
      },
      { timeout: 90000 },
    )
    .toBe("Automation package report");
  const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
    automation_id: automation.id,
  });
  const run = runs.find((item) => item.task_id === fired.run_task_id);
  expect(run?.session_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${run!.session_id}/mcp-built-in`,
        );
        const view = (await response.json()) as BuiltinMCPSessionView;
        return view.bound?.manifest.packages;
      },
      { timeout: 30000 },
    )
    .toEqual(["configuration", "core", "diagnostics", "rich-output", "walkthroughs"]);
  await testPage.goto(`/automations/${automation.id}?run=${run!.id}`);
  await expect(testPage.getByTestId("run-transcript")).toBeVisible();
  await expect(testPage.getByTestId("automation-mcp-evidence")).toHaveCount(0);
  saved = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    mcp_selection: { builtin_packages: ["core"], definition_ids: [] },
    expected_mcp_selection_revision: saved.mcp_selection_revision,
  });
  const latest = await apiClient.wsRequest<Automation>("automation.get", { id: automation.id });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    task_mode: "normal_task",
    workflow_id: seedData.workflowId,
    expected_mcp_selection_revision: latest.mcp_selection_revision,
  });
  const retainedResponse = await apiClient.rawRequest(
    "GET",
    `/api/v1/task-sessions/${run!.session_id}/mcp-built-in`,
  );
  expect(retainedResponse.ok).toBe(true);
  const retained = (await retainedResponse.json()) as BuiltinMCPSessionView;
  expect(retained.history?.current).toBeTruthy();
  await testPage.reload();
  await expect(testPage.getByTestId("run-transcript")).toBeVisible();
  await expect(testPage.getByTestId("automation-mcp-evidence")).toHaveCount(0);
  await testPage.screenshot({
    path: "../../artifacts/mcp-switches/automation-run-without-summary.png",
  });
  await testPage.setViewportSize({ width: 393, height: 851 });
  await expect(testPage.getByTestId("runs-drawer-trigger")).toBeVisible();
  await expect(testPage.getByTestId("run-transcript")).toBeVisible();
  await expect(testPage.getByTestId("automation-mcp-evidence")).toHaveCount(0);
  await apiClient.deleteMCPServer(seedData.workspaceId, server.id, server.revision);
});

test("retains an MCP draft after a stale save and reconciles only after reload", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const a = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Concurrent MCP edit",
    repositoryMode: "none",
  });
  const watcher = watchWs(testPage);
  await testPage.goto(`/settings/workspaces/${seedData.workspaceId}/automations/${a.id}`);
  await chooseAutomationPackage(testPage, "configuration", true);
  await apiClient.wsRequest("automation.update", {
    id: a.id,
    mcp_selection: { builtin_packages: ["core", "diagnostics"], definition_ids: [] },
    expected_mcp_selection_revision: 0,
  });
  await testPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: "Save changes", exact: true })
    .click();
  const section = testPage.getByTestId("automation-mcp-access");
  await expect(section.getByRole("alert")).toContainText("Choices changed");
  await expect(
    testPage.getByTestId("automation-mcp-choice-configuration").getByRole("switch"),
  ).toBeChecked();
  await section.getByRole("button", { name: "Reload choices", exact: true }).click();
  await expect(section.getByRole("alert")).toHaveCount(0);
  await expect(
    testPage.getByTestId("automation-mcp-choice-configuration").getByRole("switch"),
  ).toBeChecked();
  await saveAutomationMCP(testPage, watcher);
  const saved = await apiClient.wsRequest<Automation>("automation.get", { id: a.id });
  expect(saved.mcp_selection?.builtin_packages).toEqual(["core", "configuration"]);
});

test("applies a normal-task MCP template before its first process starts", async ({
  apiClient,
  seedData,
}) => {
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Normal task MCP template",
    workflowId: seedData.workflowId,
    taskMode: "normal_task",
    repositoryMode: "selected",
    repositories: [{ repository_id: seedData.repositoryId, base_branch: "main" }],
    agentProfileId: seedData.agentProfileId,
    executorProfileId: seedData.worktreeExecutorProfileId,
    prompt:
      'e2e:mcp:kandev-configuration:list_shared_prompts_kandev({})\ne2e:message("template-ready")',
  });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    continuation_policy: "reuse_thread",
    max_concurrent_runs: 1,
    mcp_selection: { builtin_packages: ["core", "configuration"], definition_ids: [] },
    expected_mcp_selection_revision: 0,
  });
  const fired = await apiClient.triggerAutomationManual(automation.id);
  expect(fired.run_task_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
          automation_id: automation.id,
        });
        const run = runs.find((item) => item.task_id === fired.run_task_id);
        if (run?.error_message) throw new Error(run.error_message);
        const { sessions } = await apiClient.listTaskSessions(fired.run_task_id!);
        const sessionId = run?.session_id || sessions[0]?.id;
        if (!sessionId) return [];
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${sessionId}/mcp-built-in`,
        );
        const view = (await response.json()) as BuiltinMCPSessionView;
        return view.bound?.manifest.packages;
      },
      { timeout: 60000 },
    )
    .toEqual(["configuration", "core"]);
  await expect
    .poll(
      async () => {
        const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
          automation_id: automation.id,
        });
        return runs.find((run) => run.task_id === fired.run_task_id)?.status;
      },
      { timeout: 30000 },
    )
    .toBe("succeeded");
  const getChoices = async () => {
    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/tasks/${fired.run_task_id}/mcp-built-in`,
    );
    return (await response.json()) as import("@/lib/types/builtin-mcp").BuiltinMCPManagementView;
  };
  const initial = await getChoices();
  const edited = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${fired.run_task_id}/mcp-built-in/preferences`,
    {
      version: 1,
      expected_revision: initial.preferences?.revision ?? 0,
      expected_workspace_policy_revision: initial.policy.revision,
      packages: {
        configuration: "off",
        "rich-output": "off",
        walkthroughs: "off",
        diagnostics: "off",
      },
    },
  );
  expect(edited.ok).toBe(true);
  const reused = await apiClient.triggerAutomationManual(automation.id);
  expect(reused.run_task_id).toBe(fired.run_task_id);
  expect((await getChoices()).desired?.packages).toEqual(["core"]);
});

test("keeps a running private binding until the next process starts", async ({
  apiClient,
  seedData,
}) => {
  test.setTimeout(90000);
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Running MCP binding",
    repositoryMode: "none",
    agentProfileId: seedData.agentProfileId,
    executorProfileId: seedData.worktreeExecutorProfileId,
    prompt: 'e2e:delay(8000)\ne2e:message("retained-mcp-ready")',
  });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    continuation_policy: "reuse_thread",
    max_concurrent_runs: 1,
    mcp_selection: { builtin_packages: ["core", "rich-output"], definition_ids: [] },
    expected_mcp_selection_revision: 0,
  });
  const first = await apiClient.triggerAutomationManual(automation.id);
  const currentRun = async (previousId?: string) => {
    const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
      automation_id: automation.id,
    });
    return runs.find((run) => run.task_id === first.run_task_id && run.id !== previousId);
  };
  await expect.poll(async () => (await currentRun())?.session_id, { timeout: 30000 }).toBeTruthy();
  const firstRun = (await currentRun())!;
  const view = async () => {
    const response = await apiClient.rawRequest(
      "GET",
      `/api/v1/task-sessions/${firstRun.session_id}/mcp-built-in`,
    );
    return (await response.json()) as BuiltinMCPSessionView;
  };
  await expect
    .poll(async () => (await view()).bound?.manifest.packages, { timeout: 30000 })
    .toEqual(["core", "rich-output"]);
  const accepted = (await view()).bound!;
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    mcp_selection: { builtin_packages: ["core"], definition_ids: [] },
    expected_mcp_selection_revision: 1,
  });
  const pending = await view();
  expect(pending.bound?.execution_id).toBe(accepted.execution_id);
  expect(pending.bound?.manifest.packages).toEqual(["core", "rich-output"]);
  expect(pending.desired?.packages).toEqual(["core"]);
  await expect.poll(async () => (await currentRun())?.status, { timeout: 30000 }).toBe("succeeded");
  const second = await apiClient.triggerAutomationManual(automation.id);
  expect(second.run_task_id).toBe(first.run_task_id);
  await expect
    .poll(async () => (await currentRun(firstRun.id))?.status, { timeout: 30000 })
    .toBe("succeeded");
  expect((await currentRun(firstRun.id))?.session_id).toBe(firstRun.session_id);
  expect((await view()).bound?.execution_id).not.toBe(accepted.execution_id);
  expect((await view()).bound?.manifest.packages).toEqual(["core"]);
});

test(
  "removes blocked and inherited direct MCP choices without losing inheritance",
  verifyAutomationMCPRecovery,
);

test("starts a normal task with a deleted template server recorded as unavailable", async ({
  apiClient,
  seedData,
}) => {
  test.setTimeout(90000);
  const server = await apiClient.createMCPServer(seedData.workspaceId, {
    runtime_name: "retired-reports",
    display_name: "Retired reports",
    execution_mode: "remote",
    transport: "streamable_http",
    configuration: { url: "https://reports.example.test/mcp" },
    source: "custom",
  });
  const automation = await apiClient.seedAutomation({
    workspaceId: seedData.workspaceId,
    name: "Unavailable template server",
    workflowId: seedData.workflowId,
    taskMode: "normal_task",
    repositoryMode: "none",
    agentProfileId: seedData.agentProfileId,
    executorProfileId: seedData.worktreeExecutorProfileId,
    prompt: 'e2e:message("unavailable-template-ready")',
  });
  await apiClient.wsRequest("automation.update", {
    id: automation.id,
    mcp_selection: { builtin_packages: ["core"], definition_ids: [server.id] },
    expected_mcp_selection_revision: 0,
  });
  await apiClient.deleteMCPServer(seedData.workspaceId, server.id, server.revision);
  const fired = await apiClient.triggerAutomationManual(automation.id);
  expect(fired.run_task_id).toBeTruthy();
  await expect
    .poll(
      async () => {
        const runs = await apiClient.wsRequest<AutomationRun[]>("automation.runs.list", {
          automation_id: automation.id,
        });
        const run = runs.find((item) => item.task_id === fired.run_task_id);
        if (run?.error_message) throw new Error(run.error_message);
        if (!run?.session_id) return undefined;
        const response = await apiClient.rawRequest(
          "GET",
          `/api/v1/task-sessions/${run.session_id}/mcp-built-in`,
        );
        const view = (await response.json()) as BuiltinMCPSessionView;
        return view.history?.current.servers.find((item) => item.definition_id === server.id)
          ?.reason_code;
      },
      { timeout: 60000 },
    )
    .toBe("definition_missing");
});

test("warns before deleting a server selected by an automation", async ({
  testPage,
  apiClient,
  seedData,
  prCapture,
}) => {
  await verifyAutomationMCPDeleteWarning({ testPage, apiClient, seedData, prCapture });
});
