import type { SandboxReport } from "@/lib/api/domains/sandbox-reports-api";
import { expect, test } from "../fixtures/test-base";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow } from "./layout-assertions";
import { waitForFiniteAnimations } from "./pr-capture";

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
  typedReportsScenario();
  test("reads bounded denial evidence and preserves it after a failed refresh", async ({
    testPage,
    apiClient,
    seedData,
    prCapture,
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
    await testPage.getByRole("button", { name: "Reports", exact: true }).click();
    const dialog = testPage.getByRole("dialog", { name: "Reports", exact: true });
    const section = dialog.getByTestId("sandbox-denial-report");
    const refresh = section.getByRole("button", { name: "Refresh report", exact: true });
    const writes: string[] = [];
    testPage.on("request", (request) => {
      if (
        request.url().includes("/api/v1/sandbox/") &&
        request.method() !== "GET" &&
        new URL(request.url()).pathname !== "/api/v1/sandbox/preview"
      )
        writes.push(request.url());
    });
    await expect(section.getByText("Read denied", { exact: true })).toBeVisible();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    await expect(
      section.getByText("This report is incomplete. Other denied operations may be absent.", {
        exact: true,
      }),
    ).toBeVisible();
    await expect(section.getByText("Reported classes: Filesystem", { exact: true })).toBeVisible();
    await section.getByRole("checkbox").check();
    const hide = section.getByRole("button", { name: "Hide selected denials", exact: true });
    await hide.click();
    const show = section.getByRole("button", { name: "Show hidden denials", exact: true });
    await expect(show).toBeFocused();
    await expect(
      section.getByText(report.data.records![0].target, { exact: true }),
    ).not.toBeAttached();
    await expect(
      section.getByRole("button", { name: "Review selected denials", exact: true }),
    ).toBeDisabled();
    if (prCapture.capturing) await waitForFiniteAnimations(dialog);
    await prCapture.screenshot(mobile ? "hidden-denials-phone" : "hidden-denials-desktop", {
      caption:
        "Denial notices are hidden only in this view. Audit evidence and policy remain unchanged.",
    });
    report = {
      ...report,
      version: 2,
      digest: "a".repeat(64),
      data: { ...report.data, records: [{ ...report.data.records![0], id: "repeated" }] },
    };
    await refresh.click();
    await expect(
      section.getByText("1 denial is hidden in this view.", { exact: true }),
    ).toBeVisible();
    await expect(
      section.getByText(report.data.records![0].target, { exact: true }),
    ).not.toBeAttached();
    await show.click();
    await expect(section.getByRole("checkbox").first()).toBeFocused();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    expect(writes).toEqual([]);
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await refresh.scrollIntoViewIfNeeded();
        expect((await refresh.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        expect((await hide.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        expect((await show.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await assertNoDocumentHorizontalOverflow(testPage, `report at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    await refresh.scrollIntoViewIfNeeded();
    await dialog.screenshot({ path: testInfo.outputPath("sandbox-report.png") });
    await section.getByRole("checkbox").check();
    await hide.click();
    await dialog.getByRole("button", { name: mobile ? "Back" : "Close", exact: true }).click();
    if (mobile)
      await testPage
        .getByRole("dialog", { name: "Execution details", exact: true })
        .getByRole("button", { name: "Back", exact: true })
        .click();
    await execution.click();
    await testPage.getByRole("button", { name: "Reports", exact: true }).click();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    expect(writes).toEqual([]);
    const before = reads;
    failed = true;
    await refresh.click();
    await expect(section.getByRole("alert")).toBeVisible();
    await expect(section.getByText(report.data.records![0].target, { exact: true })).toBeVisible();
    expect(reads).toBe(before + 1);
    failed = false;
    report = {
      ...report,
      version: 3,
      digest: "b".repeat(64),
      data: { ...report.data, finalized: true, records: [] },
    };
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
    if (mobile)
      await testPage
        .getByRole("dialog", { name: "Execution details", exact: true })
        .getByRole("button", { name: "Back", exact: true })
        .click();
    await expect(execution).toBeFocused();
  });
}

function typedReportsScenario() {
  test("filters typed events and preserves original evidence after a failed collection", async ({
    testPage,
    apiClient,
    seedData,
  }, testInfo) => {
    const mobile = testInfo.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Typed sandbox reports", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    let report = typedReportFixture(sessionId);
    const writes: string[] = [];
    await testPage.route("**/api/v1/sandbox/**", async (route) => {
      if (
        route.request().method() !== "GET" &&
        new URL(route.request().url()).pathname !== "/api/v1/sandbox/preview"
      )
        writes.push(route.request().url());
      if (new URL(route.request().url()).pathname.endsWith("/report"))
        await route.fulfill({ json: report });
      else await route.fulfill({ status: 404, json: {} });
    });
    await testPage.goto(`/t/${task.id}`);
    await projectExecutionFixture(testPage, sessionId);
    await enableSandboxSettingsFixture(testPage);
    await testPage.getByTestId(`execution-control-${sessionId}`).click();
    await testPage.getByRole("button", { name: "Reports", exact: true }).click();
    const dialog = testPage.getByRole("dialog", { name: "Reports", exact: true });
    const events = dialog.getByTestId("sandbox-report-event");
    await expect(events).toHaveCount(4);
    await expect(dialog.getByText("Valid event chain", { exact: true })).toBeVisible();
    await expect(dialog.getByText("Live", { exact: true })).toBeVisible();
    await expect(dialog.getByText("Partial coverage", { exact: true })).toHaveCount(4);
    await expect(dialog.getByRole("checkbox")).toHaveCount(1);
    await dialog.getByRole("checkbox").check();
    await dialog.getByRole("button", { name: "Review selected denials", exact: true }).click();
    const review = testPage.getByRole("dialog", { name: "Review policy changes", exact: true });
    await expect(testPage.getByRole("dialog")).toHaveCount(1);
    await expect(review.getByLabel("Policy name", { exact: true })).toBeVisible();
    await review.getByRole("button", { name: "Back", exact: true }).click();
    await expect(
      dialog.getByRole("button", { name: "Review selected denials", exact: true }),
    ).toBeFocused();
    await dialog.getByRole("button", { name: "Hide selected denials", exact: true }).click();
    await expect(events).toHaveCount(3);
    await dialog.getByRole("button", { name: "Show hidden denials", exact: true }).click();
    await expect(events).toHaveCount(4);
    await expect(dialog.getByRole("checkbox")).toBeFocused();
    await dialog.getByRole("combobox", { name: "Operation kind", exact: true }).click();
    await testPage.getByRole("option", { name: "Network", exact: true }).click();
    await expect(events).toHaveCount(1);
    await expect(
      events.getByText(
        "The target identity is incomplete or redacted. Policy learning is unavailable.",
        { exact: true },
      ),
    ).toBeVisible();
    await expect(dialog.getByRole("checkbox")).toHaveCount(0);
    await events.getByText("Event details", { exact: true }).click();
    await expect(events.getByText("Source ID", { exact: true })).toBeVisible();
    await dialog.getByRole("combobox", { name: "Decision", exact: true }).click();
    await testPage.getByRole("option", { name: "Allowed", exact: true }).click();
    await expect(events).toHaveCount(0);
    await expect(dialog.getByText("No events match these filters.", { exact: true })).toBeVisible();
    await dialog.getByRole("combobox", { name: "Operation kind", exact: true }).click();
    await testPage.getByRole("option", { name: "All", exact: true }).click();
    await expect(events).toHaveCount(1);
    await dialog.getByRole("combobox", { name: "Decision", exact: true }).click();
    await testPage.getByRole("option", { name: "All", exact: true }).click();
    await expect(events).toHaveCount(4);
    const originalRead = report.data.last_successful_read_at;
    report = {
      ...report,
      version: 2,
      digest: "9".repeat(64),
      captured_at: "2026-11-02T14:00:00Z",
      data: { ...report.data, status: "unavailable", refresh_error: "invalid_evidence" },
    };
    await dialog.getByRole("button", { name: "Refresh report", exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText(
      "Refresh failed. These records are from the last successful read.",
    );
    await expect(
      dialog.getByText(`Last successful read: ${originalRead}`, { exact: true }),
    ).toBeVisible();
    await expect(events).toHaveCount(4);
    const back = dialog.getByRole("button", { name: "Back", exact: true });
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        const body = dialog.getByTestId("sandbox-denial-report");
        await body.evaluate((element) => {
          element.scrollTop = element.scrollHeight;
        });
        await expect(back).toBeInViewport();
        expect((await back.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        for (const control of await dialog.getByRole("combobox").all())
          expect((await control.boundingBox())!.height).toBeGreaterThanOrEqual(44);
        await assertNoDocumentHorizontalOverflow(testPage, `typed report at ${width}`);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    if (!mobile) {
      await testPage.setViewportSize({ width: 768, height: 851 });
      if (!(await dialog.isVisible())) {
        await testPage.getByTestId(`execution-control-${sessionId}`).click();
        await testPage.getByRole("button", { name: "Reports", exact: true }).click();
      }
      expect((await back.boundingBox())!.height).toBe(28);
      await assertNoDocumentHorizontalOverflow(
        testPage,
        "typed report at 768px with a fine pointer",
      );
    }
    await dialog.getByTestId("sandbox-denial-report").evaluate((element) => {
      element.scrollTop = 0;
    });
    expect(
      await dialog.evaluate((element) => element.scrollTop),
      "the dialog shell must not scroll its fixed header",
    ).toBe(0);
    await expect(back).toBeInViewport({ ratio: 1 });
    await expect(dialog.getByRole("heading", { name: "Reports", exact: true })).toBeInViewport({
      ratio: 1,
    });
    const frame = (await dialog.boundingBox())!;
    expect(frame.y).toBeGreaterThanOrEqual(0);
    expect(frame.y + frame.height).toBeLessThanOrEqual(testPage.viewportSize()!.height);
    await testPage.screenshot({
      path: testInfo.outputPath("sandbox-typed-report.png"),
      clip: frame,
    });
    await expect(back).toBeInViewport({ ratio: 1 });
    expect(writes).toEqual([]);
    await back.click();
    await expect(testPage.getByRole("button", { name: "Reports", exact: true })).toBeFocused();
    report = {
      ...report,
      version: 3,
      digest: "8".repeat(64),
      data: {
        ...report.data,
        v2: undefined,
        sources: [],
        records: [],
        last_successful_read_at: undefined,
        refresh_error: "unsupported",
      },
    };
    await testPage.getByRole("button", { name: "Reports", exact: true }).click();
    await expect(dialog.getByRole("alert")).toHaveText("The report could not be read.");
    await expect(dialog.getByText("No successful read recorded.", { exact: true })).toBeVisible();
    await expect(
      dialog.getByText("Capture from a live attempt. More records can arrive.", { exact: true }),
    ).not.toBeAttached();
    await expect(
      dialog.getByText("Refresh failed. These records are from the last successful read.", {
        exact: true,
      }),
    ).not.toBeAttached();
  });
}

function typedReportFixture(sessionId: string): SandboxReport {
  const report = reportFixture(sessionId);
  const denial = report.data.records![0];
  const base = {
    id: "filesystem",
    source_id: "source",
    provider_record_id: "request",
    sequence: 1,
    kind: "filesystem" as const,
    decision: "denied" as const,
    reason_code: "permission_denied",
    occurred_at: denial.occurred_at,
    exact_target: true,
    learning_blocker: "" as const,
    legacy_record_id: denial.id,
    operation: { filesystem: { path: denial.target, access: "read" } },
  };
  return {
    ...report,
    data: {
      ...report.data,
      last_successful_read_at: report.captured_at,
      v2: {
        schema_version: 2,
        artifact_set_digest: "f".repeat(64),
        integrity: "valid",
        finalization: "live",
        coverage: (["filesystem", "network", "command", "http_endpoint"] as const).map((kind) => ({
          kind,
          coverage: "partial" as const,
        })),
        sources: [],
        records: [
          base,
          {
            ...base,
            id: "network",
            legacy_record_id: undefined,
            kind: "network",
            exact_target: false,
            learning_blocker: "incomplete_identity",
            operation: { tcp: { host: "", port: 443, transport: "tcp" } },
          },
          {
            ...base,
            id: "command",
            legacy_record_id: undefined,
            kind: "command",
            exact_target: false,
            learning_blocker: "incomplete_identity",
            operation: { command: { id: "deploy", caller: "agent" } },
          },
          {
            ...base,
            id: "endpoint",
            legacy_record_id: undefined,
            kind: "http_endpoint",
            decision: "allowed",
            exact_target: false,
            learning_blocker: "incomplete_identity",
            operation: {
              http_endpoint: {
                route_id: "",
                origin: "",
                method: "GET",
                path: "/documents/" + "long-section/".repeat(30),
              },
            },
          },
        ],
      },
    },
  };
}
