import { expect, test } from "../fixtures/test-base";
import type { SandboxLearningProposal } from "@/lib/api/domains/sandbox-learning-api";
import { reportFixture } from "./sandbox-reports";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

function learningFixture(sessionId: string): SandboxLearningProposal {
  const report = reportFixture(sessionId);
  return {
    id: "reviewed-proposal",
    schema_version: 1,
    attempt_id: report.attempt_id,
    session_id: sessionId,
    report_digest: report.digest,
    report_version: report.version,
    report_sources: report.data.sources!,
    report_integrity: report.data.integrity,
    report_complete: false,
    base_revision_id: report.revision_id,
    base_digest: report.policy_digest,
    ceiling_revision_id: "ui-ceiling",
    ceiling_digest: report.ceiling_digest,
    provider: report.provider,
    executor_id: "ui-executor",
    binding_version: 1,
    selected_records: report.data.records!,
    added_grants: {
      filesystem: [{ path: report.data.records![0].target, writable: false }],
      network: [],
      kandev_mcp: false,
    },
    policy: {
      id: "learned-policy",
      name: "Reviewed access",
      workspace_id: "workspace",
      installation_id: report.provider.installation_id,
      provider_key: report.provider.key,
      grants: { filesystem: [], network: [], kandev_mcp: false },
      sources: [],
    },
    operator_id: "operator",
    created_at: report.captured_at,
    digest: "a".repeat(64),
  };
}

export function sandboxLearningScenario() {
  test("reviews denial grants before publication and returns focus after publication", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Sandbox policy learning", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const proposal = learningFixture(sessionId);
    const writes: { path: string; body: unknown }[] = [];
    await testPage.route("**/api/v1/sandbox/**", async (route) => {
      const request = route.request();
      const path = new URL(request.url()).pathname;
      if (request.method() === "POST") writes.push({ path, body: request.postDataJSON() });
      if (path.endsWith("/report")) await route.fulfill({ json: reportFixture(sessionId) });
      else if (path.endsWith("/learning")) await route.fulfill({ status: 201, json: proposal });
      else if (path.endsWith("/publish"))
        await route.fulfill({ status: 201, json: { id: "published", policy: proposal.policy } });
      else await route.fulfill({ status: 404, json: {} });
    });
    await testPage.goto(`/t/${task.id}`);
    await projectExecutionFixture(testPage, sessionId);
    await enableSandboxSettingsFixture(testPage);
    await testPage.getByTestId(`execution-control-${sessionId}`).click();
    const section = testPage.getByTestId("sandbox-denial-report");
    await section.getByRole("checkbox").check();
    const trigger = section.getByRole("button", { name: "Review selected denials", exact: true });
    await trigger.click();
    const review = testPage.getByRole("dialog", { name: "Review policy changes", exact: true });
    await review.getByLabel("Policy name", { exact: true }).fill(proposal.policy.name);
    await review.getByRole("button", { name: "Review changes", exact: true }).click();
    const publish = review.getByRole("button", { name: "Create revision", exact: true });
    await expect(publish).toBeEnabled();
    expect(writes).toHaveLength(1);
    expect(writes[0].body).toEqual({
      name: proposal.policy.name,
      scope: "workspace",
      record_ids: ["record"],
      report_digest: proposal.report_digest,
    });
    await expect(review.getByText(proposal.base_digest, { exact: true })).toBeVisible();
    await expect(review.getByText(proposal.ceiling_digest, { exact: true })).toBeVisible();
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        expect((await publish.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await expect(review.getByRole("button", { name: "Back", exact: true })).toBeVisible();
        await assertNoDocumentHorizontalOverflow(testPage, `learning at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    await review.screenshot({ path: testInfo.outputPath("sandbox-learning.png") });
    await publish.click();
    await expect(
      review.getByText("Created policy revision: Reviewed access", { exact: true }),
    ).toBeVisible();
    expect(writes).toHaveLength(2);
    expect(writes[1].body).toEqual({ expected_digest: proposal.digest });
    await review.getByRole("button", { name: "Close", exact: true }).last().click();
    await expect(
      section.getByRole("button", { name: "View created revision", exact: true }),
    ).toBeFocused();
    expect(writes.filter((write) => write.path.includes("/bindings/"))).toHaveLength(0);
  });
}
