import { createHash, generateKeyPairSync } from "node:crypto";
import type { Locator, Page } from "@playwright/test";
import type { AppState } from "@/lib/state/store";
import type { SandboxAuditTrust, SandboxAuditView } from "@/lib/api/domains/sandbox-audit-api";
import { expect, test } from "../fixtures/test-base";
import { waitForFiniteAnimations } from "./pr-capture";
import { dwell } from "./causal-waits";
import { projectExecutionFixture } from "./execution-details";
import { enableSandboxSettingsFixture } from "./execution-settings";
import { assertNoDocumentHorizontalOverflow, requireBox } from "./layout-assertions";
import { reportFixture } from "./sandbox-reports";

const finished = "2026-10-25T12:02:00Z";
const keyLabel = "Expected signer public key (PEM)";
const verifyLabel = "Verify audit";
const trustSaveLabel = "Trust this key";
const currentTrustLabel = "Not established by the current operator configuration";
const revisionID = "f7f6ceea-a686-4e8e-8611-cc3c8d53d422";

function publicKeyFixture() {
  const { publicKey } = generateKeyPairSync("ec", { namedCurve: "prime256v1" });
  return {
    pem: String(publicKey.export({ type: "spki", format: "pem" })),
    fingerprint: createHash("sha256")
      .update(publicKey.export({ type: "spki", format: "der" }))
      .digest("hex"),
  };
}

function reviseAuditTrust(
  trust: SandboxAuditTrust,
  revoke: boolean,
  selectedKey: ReturnType<typeof publicKeyFixture>,
): SandboxAuditTrust {
  return {
    ...trust,
    id: revisionID,
    version: trust.version + 1,
    revoked: revoke,
    public_key_pem: revoke ? "" : selectedKey.pem,
    fingerprint: revoke ? "" : selectedKey.fingerprint,
    source: "operator",
    operator_id: "operator",
    created_at: finished,
    digest: "b".repeat(64),
  };
}

async function installAuditFixture(page: Page, sessionId: string, workspaceId: string) {
  const report = reportFixture(sessionId);
  report.provider.protocol_major = 2;
  report.data.finalized = true;
  report.data.final_audit = {
    binding_digest: "f".repeat(64),
    artifact_set_digest: "a".repeat(64),
    captured_at: finished,
  };
  let trust: SandboxAuditTrust = {
    schema_version: 1,
    id: "",
    version: 0,
    org_id: "org",
    workspace_id: workspaceId,
    installation_id: report.provider.installation_id,
    provider_key: report.provider.key,
    fingerprint: "",
    public_key_pem: "",
    revoked: true,
    operator_id: "",
    source: "unconfigured",
    created_at: finished,
    digest: "",
  };
  let job: SandboxAuditView | null = null;
  let writes = 0;
  let verifies = 0;
  let failed = false;
  const selectedKey = publicKeyFixture();
  await page.route("**/api/v1/sandbox/**", async (route) => {
    const request = route.request();
    const path = new URL(request.url()).pathname;
    if (request.method() !== "GET" && !path.endsWith("/preview")) writes++;
    if (failed) return route.fulfill({ status: 503, json: {} });
    if (path.endsWith("/report")) return route.fulfill({ json: report });
    if (path.endsWith("/audit/trust")) {
      if (request.method() === "PUT") {
        const input = request.postDataJSON();
        expect(input.expected_version).toBe(trust.version);
        if (!input.revoke) expect(input.public_key_pem).toBe(selectedKey.pem);
        trust = reviseAuditTrust(trust, Boolean(input.revoke), selectedKey);
      }
      return route.fulfill({ json: trust });
    }
    if (path.endsWith("/audit/verifications")) {
      if (request.method() === "POST") {
        verifies++;
        const input = request.postDataJSON();
        expect(input.expected_report_digest).toBe(report.digest);
        expect(input.expected_artifact_digest).toBe(report.data.final_audit!.artifact_set_digest);
        expect(input.expected_trust_version).toBe(trust.version);
        job = {
          signer_trusted_now: false,
          verification: {
            schema_version: 1,
            id: input.request_id,
            version: 1,
            state: "pending",
            attempt_id: report.attempt_id,
            session_id: sessionId,
            provider_session_id: "captured-provider-session",
            report_digest: report.digest,
            artifact_set_digest: report.data.final_audit!.artifact_set_digest,
            binding_digest: report.data.final_audit!.binding_digest,
            provider: report.provider,
            trust: {
              revision_id: trust.id,
              version: trust.version,
              fingerprint: trust.fingerprint,
              revoked: trust.revoked,
              source: trust.source,
              operator_id: trust.operator_id,
              configured_at: trust.created_at,
            },
            requested_by: "operator",
            requested_at: "2026-10-25T12:01:00Z",
          },
        };
        return route.fulfill({ status: 202, json: job });
      }
      return route.fulfill({
        json: { verifications: job ? [{ ...job, signer_trusted_now: !trust.revoked }] : [] },
      });
    }
    if (job && path.endsWith(`/audit/verifications/${job.verification.id}`)) {
      job = {
        signer_trusted_now: !trust.revoked,
        verification: {
          ...job.verification,
          version: 3,
          state: "completed",
          started_at: "2026-10-25T12:01:01Z",
          completed_at: finished,
          result: {
            artifact_set_digest: report.data.final_audit!.artifact_set_digest,
            verifier_version: "fixture-verifier-1",
            session_id: job.verification.provider_session_id,
            event_root: "c".repeat(64),
            event_count: 17,
            chain_state: "valid",
            finalization: "final",
            signature_state: trust.revoked ? "unsigned" : "verified",
            expected_key_fingerprint: trust.fingerprint,
            signer_key_fingerprint: trust.fingerprint,
            ledger_state: "valid",
            trust_revision: trust.id,
          },
        },
      };
      return route.fulfill({ json: job });
    }
    return route.fulfill({ status: 404, json: {} });
  });
  return {
    selectedKey,
    get writes() {
      return writes;
    },
    get verifies() {
      return verifies;
    },
    setFailed: (value: boolean) => {
      failed = value;
    },
    changeTrust: () => {
      trust = { ...trust, version: trust.version + 1, id: revisionID, source: "operator" };
    },
  };
}

async function openAudit(page: Page, sessionId: string, taskId: string) {
  await page.goto(`/t/${taskId}`);
  await projectExecutionFixture(page, sessionId);
  await page.evaluate((id) => {
    const state = (
      window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
    ).__KANDEV_E2E_STORE__.getState();
    const session = state.taskSessions.items[id];
    const previous = session.execution_observation!;
    state.upsertTaskSessionFromEvent(session.task_id, {
      ...session,
      execution_observation: {
        ...previous,
        revision: previous.revision + 1,
        status: "stopped",
        valid_until: undefined,
        facts: {
          ...previous.facts!,
          process: { state: "stopped", checked_at: previous.checked_at },
          audit: { integrity: "unchecked", finalization: "final", signer: "unchecked" },
        },
      },
    });
  }, sessionId);
  await enableSandboxSettingsFixture(page);
  return reopenAudit(page, sessionId);
}

async function reopenAudit(page: Page, sessionId: string) {
  const trigger = page.getByTestId(`execution-control-${sessionId}`);
  await expect(trigger).toHaveCount(1);
  await trigger.click();
  await page.getByRole("button", { name: "Reports", exact: true }).click();
  await page
    .getByRole("dialog")
    .getByRole("button", { name: "Audit verification", exact: true })
    .click();
  return page.getByRole("dialog", { name: "Audit verification", exact: true });
}

async function checkGeometry(page: Page, dialog: Locator, mobile: boolean) {
  await waitForFiniteAnimations(dialog);
  const action = dialog.getByRole("button", { name: verifyLabel, exact: true });
  const box = await requireBox(dialog, "audit dialog");
  const button = await requireBox(action, "verify action");
  const viewport = page.viewportSize()!;
  expect(box.x).toBeGreaterThanOrEqual(-1);
  expect(box.y).toBeGreaterThanOrEqual(-1);
  expect(box.x + box.width).toBeLessThanOrEqual(viewport.width + 1);
  expect(box.y + box.height).toBeLessThanOrEqual(viewport.height + 1);
  expect(button.y + button.height).toBeLessThanOrEqual(box.y + box.height);
  if (mobile) expect(button.height).toBeGreaterThanOrEqual(44);
  else expect(button.height).toBeLessThanOrEqual(30);
  expect(await dialog.evaluate((el) => el.scrollTop)).toBe(0);
  expect(
    await dialog.getByTestId("sandbox-audit-body").evaluate((el) => getComputedStyle(el).overflowY),
  ).toBe("auto");
  await assertNoDocumentHorizontalOverflow(page);
}

export function sandboxAuditScenario() {
  test("verifies only on request and preserves unsigned results after a failed refresh", async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    const mobile = info.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Audit verification", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const fixture = await installAuditFixture(testPage, sessionId, seedData.workspaceId);
    const dialog = await openAudit(testPage, sessionId, task.id);
    await expect(dialog).toContainText("No operator key is configured.");
    await expect(dialog.getByRole("button", { name: verifyLabel, exact: true })).toBeEnabled();
    await dwell(
      testPage,
      1100,
      "negative-assertion",
      "Opening and reading audit history must not start verification or write trust.",
    );
    expect(fixture.writes).toBe(0);
    await dialog.getByRole("button", { name: verifyLabel, exact: true }).dblclick();
    await expect(
      dialog.getByRole("heading", { name: "Verification completed", exact: true }),
    ).toBeVisible();
    expect(fixture.verifies).toBe(1);
    await expect(dialog).toContainText("Unsigned");
    await expect(dialog.getByTestId("sandbox-audit-current-trust")).toHaveText(currentTrustLabel);
    await expect(dialog).toContainText(finished);
    await dialog.getByText("Verification details", { exact: true }).click();
    await expect(dialog).toContainText("fixture-verifier-1");
    await expect(dialog).toContainText("c".repeat(64));
    await checkGeometry(testPage, dialog, mobile);
    await testPage.screenshot({
      path: info.outputPath("audit-unsigned.png"),
      clip: await requireBox(dialog, "audit dialog"),
    });
    fixture.setFailed(true);
    await dialog.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText("Saved results remain visible.");
    await expect(dialog).toContainText(finished);
    await expect(dialog).toContainText("Unsigned");
    await expect(dialog.getByRole("button", { name: verifyLabel, exact: true })).toBeDisabled();
    expect(fixture.verifies).toBe(1);
    await dialog.getByRole("button", { name: "Back", exact: true }).click();
    await expect(
      testPage.getByRole("button", { name: "Audit verification", exact: true }),
    ).toBeFocused();
    await testPage.getByRole("dialog").getByRole("button", { name: "Back", exact: true }).click();
    await expect(testPage.getByRole("button", { name: "Reports", exact: true })).toBeFocused();
  });

  test("stores explicit public-key trust, rejects stale drafts, and separates revocation from signature history", async ({
    testPage,
    apiClient,
    seedData,
  }, info) => {
    const mobile = info.project.name === "mobile-chrome";
    const task = await apiClient.createTask(seedData.workspaceId, "Audit signer trust", {
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      agent_profile_id: seedData.agentProfileId,
    });
    const { session_id: sessionId } = await apiClient.seedTaskSession(task.id, {
      state: "WAITING_FOR_INPUT",
      agentProfileId: seedData.agentProfileId,
    });
    const fixture = await installAuditFixture(testPage, sessionId, seedData.workspaceId);
    const dialog = await openAudit(testPage, sessionId, task.id);
    await expect(dialog.getByLabel(keyLabel, { exact: true })).toBeEnabled();
    await dialog
      .getByLabel(keyLabel, { exact: true })
      .fill("-----BEGIN PRIVATE KEY-----\nfixture-only\n-----END PRIVATE KEY-----");
    await expect(dialog.getByRole("button", { name: trustSaveLabel, exact: true })).toBeDisabled();
    expect(fixture.writes).toBe(0);
    await dialog.getByLabel(keyLabel, { exact: true }).fill(fixture.selectedKey.pem);
    fixture.changeTrust();
    await dialog.getByRole("button", { name: "Refresh", exact: true }).click();
    await expect(dialog.getByRole("alert")).toContainText(
      "Trust changed after you edited this key.",
    );
    await expect(dialog.getByRole("button", { name: trustSaveLabel, exact: true })).toBeDisabled();
    await expect(dialog.getByLabel(keyLabel, { exact: true })).toHaveValue(fixture.selectedKey.pem);
    expect(fixture.writes).toBe(0);
    await dialog.getByRole("button", { name: "Load current key", exact: true }).click();
    await dialog.getByLabel(keyLabel, { exact: true }).fill(fixture.selectedKey.pem);
    await dialog.getByRole("button", { name: trustSaveLabel, exact: true }).click();
    await expect(dialog).toContainText("Operator key configured. Version 2.");
    expect(fixture.verifies).toBe(0);
    await dialog.getByRole("button", { name: verifyLabel, exact: true }).click();
    await expect(
      dialog.getByRole("heading", { name: "Verification completed", exact: true }),
    ).toBeVisible();
    await expect(dialog).toContainText("Valid for the selected key");
    await expect(dialog.getByTestId("sandbox-audit-current-trust")).toHaveText(
      "Trusted by the current operator configuration",
    );
    await checkGeometry(testPage, dialog, mobile);
    await dialog.getByRole("button", { name: "Revoke trust", exact: true }).click();
    await expect(dialog).toContainText("Operator trust revoked. Version 3.");
    await expect(dialog.getByTestId("sandbox-audit-current-trust")).toHaveText(currentTrustLabel);
    await expect(dialog).toContainText("Valid for the selected key");
    await expect(dialog).toContainText(finished);
    expect(fixture.verifies).toBe(1);
    if (mobile) {
      for (const width of [393, 700, 767]) {
        await testPage.setViewportSize({ width, height: 851 });
        await checkGeometry(testPage, dialog, true);
      }
      await testPage.setViewportSize({ width: 393, height: 851 });
    }
    if (!mobile) {
      const viewport = testPage.viewportSize()!;
      await testPage.setViewportSize({ width: 768, height: 851 });
      await checkGeometry(testPage, dialog, false);
      await testPage.setViewportSize({ width: 767, height: 851 });
      await testPage.getByTestId("mobile-sessions-pill").click();
      await testPage
        .getByTestId(`mobile-session-row-${sessionId}`)
        .getByRole("button", { name: "Session actions" })
        .click();
      await testPage.getByRole("menuitem", { name: "Execution details", exact: true }).click();
      await testPage.getByRole("button", { name: "Reports", exact: true }).click();
      await testPage
        .getByRole("dialog")
        .getByRole("button", { name: "Audit verification", exact: true })
        .click();
      await checkGeometry(testPage, dialog, true);
      await testPage.setViewportSize(viewport);
      if (!(await dialog.isVisible())) await openAudit(testPage, sessionId, task.id);
    }
    await expect(dialog.getByTestId("sandbox-audit-current-trust")).toHaveText(currentTrustLabel);
    await expect(dialog).toContainText(finished);
    await testPage.screenshot({
      path: info.outputPath("audit-revoked.png"),
      clip: await requireBox(dialog, "audit dialog"),
    });
    const writesBefore = fixture.writes;
    await testPage.evaluate(() => {
      const state = (
        window as unknown as { __KANDEV_E2E_STORE__: { getState(): AppState } }
      ).__KANDEV_E2E_STORE__.getState();
      state.setAuthState({ ...state.auth, user: { ...state.auth.user!, role: "member" } });
    });
    // Authority changes discard the old scoped report and close its next view.
    await testPage
      .getByRole("dialog", { name: "Reports", exact: true })
      .getByRole("button", { name: "Audit verification", exact: true })
      .click();
    await expect(dialog).toContainText(
      "Only an operator can change trust or request verification.",
    );
    await expect(dialog.getByLabel(keyLabel, { exact: true })).toHaveCount(0);
    await expect(dialog.getByRole("button", { name: verifyLabel, exact: true })).toHaveCount(0);
    await expect(dialog).toContainText(finished);
    expect(fixture.writes).toBe(writesBefore);
  });
}
