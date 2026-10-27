import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import { expect, test } from "../fixtures/test-base";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";

export function reportFixture(sessionId: string): SandboxReport {
  return {
    schema_version: 1,
    attempt_id: "ui-attempt",
    session_id: sessionId,
    revision_id: "ui-revision",
    policy_digest: "c".repeat(64),
    ceiling_digest: "d".repeat(64),
    version: 1,
    digest: "e".repeat(64),
    captured_at: "2026-10-25T12:00:00Z",
    provider: {
      plugin_id: "fixture",
      installation_id: "ui-install",
      key: "fixture-provider",
      version: "1.0",
      package_digest: "a".repeat(64),
      runtime_version: "2.0",
      runtime_digest: "b".repeat(64),
    },
    data: {
      status: "available",
      integrity: "partial",
      complete: false,
      finalized: false,
      supported_classes: ["filesystem"],
      sources: [
        {
          id: "source",
          schema: "fixture.audit.v1",
          digest: "f".repeat(64),
          cursor: "",
          next_cursor: "",
          integrity: "partial",
          complete: false,
        },
      ],
      records: [
        {
          id: "record",
          source_id: "source",
          provider_record_id: "denial",
          kind: "filesystem",
          operation: "read",
          target: "/workspace/" + "long-directory/".repeat(25) + "denied.txt",
          occurred_at: "2026-10-25T11:59:00Z",
        },
      ],
    },
  };
}

export function sandboxReportsScenario() {
  test("reads bounded denial evidence and preserves it after a failed refresh", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Sandbox reports", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    let report = reportFixture(sessionId);
    let failed = false;
    let reads = 0;
    await testPage.route("**/api/v1/sandbox/**", async (route) => {
      if (new URL(route.request().url()).pathname.endsWith("/report")) {
        reads++;
        await route.fulfill({ status: failed ? 503 : 200, json: failed ? {} : report });
      } else await route.fulfill({ status: 404, json: {} });
    });
    await testPage.goto(`/t/${task.id}`);
    await projectExecutionFixture(testPage, sessionId);
    await enableSandboxSettingsFixture(testPage);
    const execution = testPage.getByTestId(`execution-control-${sessionId}`);
    await execution.click();
    const dialog = testPage.getByRole("dialog", { name: "Execution details", exact: true });
    const section = dialog.getByTestId("sandbox-denial-report");
    const refresh = section.getByRole("button", { name: "Refresh report", exact: true });
    await expect(section.getByText("Read denied", { exact: true })).toBeVisible();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    await expect(
      section.getByText("This report is incomplete. Other denied operations may be absent.", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(section.getByText("Reported classes: Filesystem", { exact: true })).toBeVisible();
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await refresh.scrollIntoViewIfNeeded();
        expect((await refresh.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await assertNoDocumentHorizontalOverflow(testPage, `report at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    await refresh.scrollIntoViewIfNeeded();
    await dialog.screenshot({ path: testInfo.outputPath("sandbox-report.png") });
    const before = reads;
    failed = true;
    await refresh.click();
    await expect(section.getByRole("alert")).toBeVisible();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    expect(reads).toBe(before + 1);
    failed = false;
    report = { ...report, version: 2, data: { ...report.data, finalized: true, records: [] } };
    await refresh.click();
    await expect(
      section.getByText("Final capture after process cleanup.", { exact: true }),
    ).toBeVisible();
    await expect(
      section.getByText(
        "No denial records in this capture. This does not prove that no operations were denied.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(section.getByText("Read denied", { exact: true })).not.toBeAttached();
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    await expect(execution).toBeFocused();
  });
}
