import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import {
  devices,
  expect,
  type BrowserContext,
  type Locator,
  type Page,
  type TestInfo,
} from "@playwright/test";
import { backendFixture as test } from "../../fixtures/backend";
import { acceptInvite, createInviteToken, login, setupAdmin } from "../../helpers/auth";
import { watchWs } from "../../helpers/causal-waits";
import { loadInterimSettingsInterlockToken } from "../../helpers/interim-settings-interlock";
import { replacePromptEditor } from "../../helpers/settings-prompt-editor";

const OWNER = {
  email: "automation-owner@e2e.dev",
  password: "ownerpass123",
  displayName: "Automation Owner",
};
const COLLABORATOR = {
  email: "automation-collaborator@e2e.dev",
  password: "collaboratorpass123",
  displayName: "Automation Collaborator",
};
const MEMBER = {
  email: "automation-member@e2e.dev",
  password: "memberpass123",
  displayName: "Automation Workspace Member",
};
const AUTOMATION_NAME = "Credential access review";
const REPOSITORY_NAME = "Credential review repository";
const AGENT_SECRET_KEY = "SERVICE_KEY";
const EXECUTOR_SECRET_KEY = "EXECUTOR_KEY";
const REPOSITORY_SECRET_KEY = "REPO_TOKEN";
const OWNER_GLOBAL_SECRET = "Owner global automation token";
const WORKSPACE_SECRET = "Workspace automation token";
const COLLABORATOR_SECRET = "Collaborator automation token";
const EXECUTOR_PROFILE_NAME = "Credential review Local";

type AuthUser = { id: string };
type Secret = { id: string };
type AgentProfile = { id: string; name: string };
type Agent = { id: string; name: string; profiles: AgentProfile[] };
type Executor = { id: string; type: string };
type CredentialTestBase = {
  mobile: boolean;
  testInfo: TestInfo;
  ownerContext: BrowserContext;
  collaboratorContext: BrowserContext;
  memberContext: BrowserContext;
  workspaceId: string;
  baseUrl: string;
  interlock: string;
  agentProfileName: string;
  noCredentialProfileName: string;
  workspaceSecretId: string;
};
type CredentialDraftState = CredentialTestBase & {
  automationId: string;
  collaboratorPage: Page;
  collaboratorWs: ReturnType<typeof watchWs>;
  saveButton: Locator;
};
type CredentialOwnerState = CredentialDraftState & {
  ownerPage: Page;
  ownerWs: ReturnType<typeof watchWs>;
};
type CreateSecretInput = {
  context: BrowserContext;
  baseUrl: string;
  interlock: string;
  name: string;
  scope: "global" | "workspace";
  workspaceId?: string;
};

async function userId(context: BrowserContext, baseUrl: string): Promise<string> {
  const response = await context.request.get(`${baseUrl}/api/v1/auth/me`);
  expect(response.ok(), await response.text()).toBe(true);
  return ((await response.json()) as { user: AuthUser }).user.id;
}

async function createSecret({
  context,
  baseUrl,
  interlock,
  name,
  scope,
  workspaceId,
}: CreateSecretInput): Promise<Secret> {
  const response = await context.request.post(`${baseUrl}/api/v1/secrets`, {
    headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
    data: {
      name,
      value: `${name} value`,
      scope,
      ...(workspaceId ? { workspace_id: workspaceId } : {}),
    },
  });
  expect(response.ok(), await response.text()).toBe(true);
  return (await response.json()) as Secret;
}

async function clickOrTap(locator: Locator, mobile: boolean): Promise<void> {
  if (mobile) await locator.tap();
  else await locator.click();
}

async function assertControlSize(locator: Locator, mobile: boolean): Promise<void> {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  if (mobile) expect(box!.height).toBeGreaterThanOrEqual(44);
  else expect(box!.height).toBeLessThanOrEqual(29);
}

async function selectOption(
  page: Page,
  testId: string,
  optionName: string,
  mobile: boolean,
): Promise<void> {
  const trigger = page.getByTestId(testId);
  await expect(trigger).toBeVisible();
  await clickOrTap(trigger, mobile);
  const option = page.getByRole("option").filter({ hasText: optionName }).last();
  await expect(option).toBeVisible();
  await clickOrTap(option, mobile);
  await expect(trigger).toContainText(optionName);
}

async function selectRepository(page: Page, mobile: boolean): Promise<void> {
  const add = page.getByRole("button", { name: "Add repository", exact: true });
  await clickOrTap(add, mobile);
  const trigger = page.getByTestId("repo-chip-trigger").last();
  await clickOrTap(trigger, mobile);
  const option = page.getByRole("option").filter({ hasText: REPOSITORY_NAME }).last();
  await expect(option).toBeVisible();
  await clickOrTap(option, mobile);
  await expect(page.getByTestId("repo-chip").last()).toContainText(REPOSITORY_NAME);
}

async function assertTouchTarget(locator: Locator): Promise<void> {
  const box = await locator.boundingBox();
  expect(box).not.toBeNull();
  expect(box!.height).toBeGreaterThanOrEqual(44);
}

async function expectInViewport(page: Page, locator: Locator): Promise<void> {
  await expect
    .poll(async () => {
      const box = await locator.boundingBox();
      if (!box) return false;
      const viewport = await page.evaluate(() => ({
        width: window.innerWidth,
        height: window.innerHeight,
      }));
      return (
        box.x >= 0 &&
        box.y >= 0 &&
        box.x + box.width <= viewport.width &&
        box.y + box.height <= viewport.height
      );
    })
    .toBe(true);
}

async function expectAnimationsSettled(locator: Locator): Promise<void> {
  await expect
    .poll(() =>
      locator.evaluate((element) =>
        element
          .getAnimations({ subtree: true })
          .every(
            (animation) => animation.playState === "finished" || animation.playState === "idle",
          ),
      ),
    )
    .toBe(true);
}

async function waitForSonnerToastToClear(page: Page, text: string): Promise<void> {
  await expect(page.locator("[data-sonner-toast]").filter({ hasText: text })).toHaveCount(0, {
    timeout: 5_000,
  });
}

async function waitForNotificationToastToClear(page: Page, text: string): Promise<void> {
  await expect(page.getByTestId("toast-message").filter({ hasText: text })).toHaveCount(0, {
    timeout: 5_000,
  });
}

async function capture(page: Page, testInfo: TestInfo, name: string): Promise<void> {
  const output = testInfo.outputPath(name);
  await fs.promises.mkdir(path.dirname(output), { recursive: true });
  await page.screenshot({ path: output });
}

async function createCredentialDraft(base: CredentialTestBase): Promise<CredentialDraftState> {
  const collaboratorPage = await base.collaboratorContext.newPage();
  const collaboratorWs = watchWs(collaboratorPage);
  await collaboratorPage.goto(`/settings/workspaces/${base.workspaceId}/automations/new`);
  await expect(collaboratorPage.getByTestId("automation-editor")).toBeVisible({ timeout: 15_000 });
  await collaboratorPage.getByTestId("automation-name-input").fill(AUTOMATION_NAME);
  await selectOption(
    collaboratorPage,
    "agent-profile-selector",
    base.agentProfileName,
    base.mobile,
  );
  await selectOption(
    collaboratorPage,
    "executor-profile-selector",
    EXECUTOR_PROFILE_NAME,
    base.mobile,
  );
  await selectRepository(collaboratorPage, base.mobile);
  await replacePromptEditor(
    collaboratorPage,
    collaboratorPage.getByTestId("automation-prompt-editor"),
    "/e2e:cancel-hold",
    { touch: base.mobile },
  );

  const saveButton = collaboratorPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: /save changes/i });
  await expect(saveButton).toBeVisible();
  const initialSave = collaboratorWs.waitForResponse("automation.create").then(
    () => false,
    () => true,
  );
  await clickOrTap(saveButton, base.mobile);
  expect(await initialSave, "enabled save must reject the owner's unreadable profile secret").toBe(
    true,
  );

  const credentialError = collaboratorPage.getByTestId("automation-credential-error");
  await expect(credentialError).toBeVisible();
  await expect(credentialError).toContainText(/SERVICE_KEY|EXECUTOR_KEY|REPO_TOKEN/);
  await expect(credentialError).toContainText(/Choose another profile|Choose another repository/);
  await expect(credentialError).not.toContainText(/secret-id|[0-9a-f]{8}-[0-9a-f]{4}/i);
  if (base.mobile) {
    const target = await credentialError.getAttribute("data-credential-error-target");
    expect(target).toBeTruthy();
    await expectInViewport(
      collaboratorPage,
      collaboratorPage.locator(`[data-credential-field="${target}"]`),
    );
  }
  await waitForSonnerToastToClear(collaboratorPage, "Automation credentials need attention.");
  await capture(
    collaboratorPage,
    base.testInfo,
    `${base.mobile ? "phone" : "desktop"}-credential-error.png`,
  );

  const enabled = collaboratorPage.getByTestId("automation-enabled-switch");
  await expect(enabled).toBeChecked();
  if (!base.mobile) {
    await collaboratorPage.setViewportSize({ width: 480, height: 900 });
    await expect(
      collaboratorPage.getByTestId("agent-profile-selector"),
      "profile selector remains available in a narrow fine-pointer viewport",
    ).toBeVisible();
    expect(
      await collaboratorPage.evaluate(() => window.matchMedia("(pointer: fine)").matches),
    ).toBe(true);
    expect(
      await collaboratorPage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
  }
  await expect(enabled).toBeVisible();
  await clickOrTap(enabled, base.mobile);
  await expect(enabled).not.toBeChecked();
  const draftSave = collaboratorWs.waitForResponse("automation.create");
  const retrySaveButton = collaboratorPage
    .getByTestId("settings-floating-save")
    .getByRole("button", { name: /retry save/i });
  await expect(retrySaveButton).toBeVisible();
  await clickOrTap(retrySaveButton, base.mobile);
  await draftSave;
  await expect(collaboratorPage).toHaveURL(/\/automations$/, { timeout: 15_000 });
  const automationRow = collaboratorPage
    .getByTestId(/^automation-row-/)
    .filter({ hasText: AUTOMATION_NAME });
  await expect(automationRow).toBeVisible();
  const automationId = ((await automationRow.getAttribute("data-testid")) ?? "").replace(
    "automation-row-",
    "",
  );
  expect(automationId).toMatch(/^[0-9a-f-]{36}$/i);
  await expect(
    collaboratorPage.getByTestId(`automation-enabled-${automationId}`),
  ).not.toBeChecked();
  if (!base.mobile) {
    await collaboratorPage.goto(
      `/settings/workspaces/${base.workspaceId}/automations/${automationId}`,
    );
    const savedDraftEnabled = collaboratorPage.getByTestId("automation-enabled-switch");
    await expect(savedDraftEnabled).not.toBeChecked();
    await expect(collaboratorPage.getByTestId("agent-profile-selector")).toBeVisible();
    expect(
      await collaboratorPage.evaluate(() => window.matchMedia("(pointer: fine)").matches),
    ).toBe(true);
    expect(
      await collaboratorPage.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    await savedDraftEnabled.scrollIntoViewIfNeeded();
    await capture(collaboratorPage, base.testInfo, "desktop-narrow-fine-credential-draft.png");
    await collaboratorPage.setViewportSize({ width: 1440, height: 960 });
  }

  return { ...base, automationId, collaboratorPage, collaboratorWs, saveButton };
}

async function choosePersonalSecret(state: CredentialDraftState, key: string): Promise<void> {
  const trigger = state.collaboratorPage.getByTestId(`automation-personal-secret-${key}`);
  await clickOrTap(trigger, state.mobile);
  if (state.mobile) {
    const drawer = state.collaboratorPage
      .getByRole("dialog")
      .filter({ hasText: "Choose my secret" });
    await expect(drawer).toBeVisible();
    await drawer.getByRole("option", { name: COLLABORATOR_SECRET, exact: true }).tap();
    await expect(drawer).toBeHidden();
  } else {
    const option = state.collaboratorPage.getByRole("option", {
      name: COLLABORATOR_SECRET,
      exact: true,
    });
    await expect(option).toBeVisible();
    await option.click();
  }
  await expect(trigger).toContainText(COLLABORATOR_SECRET);
  await expect(trigger).toBeFocused();
}

async function choosePersonalSecretWithKeyboard(
  state: CredentialDraftState,
  key: string,
): Promise<void> {
  const page = state.collaboratorPage;
  const trigger = page.getByTestId(`automation-personal-secret-${key}`);
  const navigationStart = page.getByTestId("automation-enabled-switch");
  await expect(trigger).toBeVisible();
  await navigationStart.focus();
  await expect(navigationStart).toBeFocused();
  let triggerReached = false;
  for (let index = 0; index < 80; index += 1) {
    await page.keyboard.press("Tab");
    if (await trigger.evaluate((element) => element === document.activeElement)) {
      triggerReached = true;
      break;
    }
  }
  expect(triggerReached, "keyboard tab order reaches the personal-secret picker").toBe(true);
  await page.keyboard.press("Enter");
  const picker = page.locator('[data-slot="popover-content"]').last();
  await expect(picker).toBeVisible();
  const search = page.getByPlaceholder("Search Global secrets");
  await expect(search).toBeFocused();
  await page.keyboard.type(COLLABORATOR_SECRET);
  const option = page.getByRole("option", { name: COLLABORATOR_SECRET, exact: true });
  await expect(option).toBeVisible();
  await page.keyboard.press("ArrowDown");
  await page.keyboard.press("Enter");
  await expect(picker).toBeHidden();
  await expect(trigger).toContainText(COLLABORATOR_SECRET);
  await expect(trigger).toBeFocused();
}

async function bindPersonalSecrets(state: CredentialDraftState): Promise<void> {
  const { collaboratorPage, mobile, workspaceId } = state;
  await collaboratorPage.goto(
    `/settings/workspaces/${workspaceId}/automations/${state.automationId}`,
  );
  const groups = collaboratorPage.getByTestId("automation-personal-credential-fields");
  await expect(groups).toHaveCount(2);
  await expect(groups.first()).toBeVisible();
  await expect(groups.nth(1)).toBeVisible();
  const personalTrigger = collaboratorPage.getByTestId(
    `automation-personal-secret-${AGENT_SECRET_KEY}`,
  );
  await assertControlSize(personalTrigger, mobile);
  await clickOrTap(personalTrigger, mobile);

  if (mobile) {
    const picker = collaboratorPage.getByRole("dialog").filter({ hasText: "Choose my secret" });
    const emptyState = picker.getByText("No Global secrets found.");
    await expect(picker).toBeVisible();
    await expect(emptyState).toBeVisible();
    await expectAnimationsSettled(picker);
    await expectInViewport(collaboratorPage, emptyState);
    await capture(collaboratorPage, state.testInfo, "phone-personal-secret-drawer.png");
    await collaboratorPage.keyboard.press("Escape");
    await expect(picker).toBeHidden();
    await expect(personalTrigger).toBeFocused();
  } else {
    const emptyState = collaboratorPage.getByText("No Global secrets found.");
    const picker = collaboratorPage.locator('[data-slot="popover-content"]').last();
    await expect(emptyState).toBeVisible();
    await expectAnimationsSettled(picker);
    await expectInViewport(collaboratorPage, emptyState);
    await capture(collaboratorPage, state.testInfo, "desktop-personal-secret-picker.png");
    await collaboratorPage.keyboard.press("Escape");
    await expect(personalTrigger).toBeFocused();
  }

  const addSecretLinks = collaboratorPage.getByRole("link", {
    name: "Add a secret in Settings > Secrets",
  });
  await expect(addSecretLinks).toHaveCount(2);
  await expect(addSecretLinks.first()).toBeVisible();
  const personalSecret = await createSecret({
    context: state.collaboratorContext,
    baseUrl: state.baseUrl,
    interlock: state.interlock,
    name: COLLABORATOR_SECRET,
    scope: "global",
  });
  expect(personalSecret.id).toBeTruthy();
  await collaboratorPage.reload();
  await expect(collaboratorPage.getByTestId("automation-editor")).toBeVisible();
  for (const key of [AGENT_SECRET_KEY, EXECUTOR_SECRET_KEY]) {
    if (!mobile && key === AGENT_SECRET_KEY) {
      await choosePersonalSecretWithKeyboard(state, key);
    } else {
      await choosePersonalSecret(state, key);
    }
  }
  const bindingSave = state.collaboratorWs.waitForResponse("automation.update");
  await clickOrTap(state.saveButton, mobile);
  await bindingSave;
  await expect(collaboratorPage.getByTestId("automation-enabled-switch")).not.toBeChecked();
}

async function grantWorkspaceSecret(state: CredentialDraftState): Promise<CredentialOwnerState> {
  const ownerPage = await state.ownerContext.newPage();
  const ownerWs = watchWs(ownerPage);
  await ownerPage.goto(
    `/settings/workspaces/${state.workspaceId}/automations/${state.automationId}`,
  );
  await expect(ownerPage.getByTestId("automation-editor")).toBeVisible();
  const workspaceCredential = ownerPage.getByTestId(
    `automation-workspace-credential-${REPOSITORY_SECRET_KEY}`,
  );
  await expect(workspaceCredential).toBeVisible();
  await expect(workspaceCredential).toContainText("Owner approval needed");
  const grantButton = ownerPage.getByTestId(
    `automation-workspace-credential-grant-${REPOSITORY_SECRET_KEY}`,
  );
  await assertControlSize(grantButton, state.mobile);
  await clickOrTap(grantButton, state.mobile);
  const confirmation = ownerPage.getByTestId(
    state.mobile
      ? "automation-workspace-credential-confirmation-drawer"
      : "automation-workspace-credential-confirmation-dialog",
  );
  await expect(confirmation).toBeVisible();
  await expect(confirmation).toContainText(WORKSPACE_SECRET);
  await expect(confirmation).toContainText(REPOSITORY_SECRET_KEY);
  await expect(confirmation).toContainText(REPOSITORY_NAME);
  await expect(confirmation).toContainText(
    "Revoking blocks new starts, retries, prompts, and runtime attachments.",
  );
  await expectAnimationsSettled(confirmation);
  const confirmGrant = confirmation.getByRole("button", { name: "Grant use", exact: true });
  if (state.mobile) await expectInViewport(ownerPage, confirmGrant);
  await assertControlSize(confirmGrant, state.mobile);
  await waitForNotificationToastToClear(ownerPage, "Kandev update available");
  await capture(
    ownerPage,
    state.testInfo,
    `${state.mobile ? "phone" : "desktop"}-workspace-grant-confirmation.png`,
  );
  const grantResponse = ownerWs.waitForResponse("automation.credential.grant_workspace");
  await clickOrTap(confirmGrant, state.mobile);
  await grantResponse;
  await expect(confirmation).toBeHidden();
  await expect(workspaceCredential).toBeFocused();
  await expect(workspaceCredential).toContainText("Approved");
  await expect(
    ownerPage.getByTestId(`automation-workspace-credential-revoke-${REPOSITORY_SECRET_KEY}`),
  ).toBeVisible();
  await waitForNotificationToastToClear(ownerPage, "Kandev update available");
  await capture(
    ownerPage,
    state.testInfo,
    `${state.mobile ? "phone" : "desktop"}-workspace-granted.png`,
  );
  return { ...state, ownerPage, ownerWs };
}

async function enableCredentialAutomation(state: CredentialDraftState): Promise<void> {
  const { collaboratorPage, mobile, automationId } = state;
  await collaboratorPage.reload();
  await expect(collaboratorPage.getByTestId("automation-editor")).toBeVisible();
  await expect(
    collaboratorPage.getByTestId(`automation-workspace-credential-${REPOSITORY_SECRET_KEY}`),
  ).toContainText("Approved");
  const enabled = collaboratorPage.getByTestId("automation-enabled-switch");
  await clickOrTap(enabled, mobile);
  await expect(enabled).toBeChecked();
  const save = state.collaboratorWs.waitForResponse("automation.update");
  await clickOrTap(state.saveButton, mobile);
  await save;
  await collaboratorPage.goto(`/automations/${automationId}`);
  await expect(collaboratorPage.getByTestId("automation-run-now")).toBeVisible({ timeout: 20_000 });
}

async function startCredentialRun(state: CredentialDraftState): Promise<string> {
  const { collaboratorPage, collaboratorWs, mobile, automationId } = state;
  const runNow = collaboratorPage.getByTestId("automation-run-now");
  if (mobile) await assertTouchTarget(runNow);
  const runResponse = collaboratorWs.waitForResponse("automation.trigger");
  await clickOrTap(runNow, mobile);
  await runResponse;
  const runs = collaboratorWs.waitForResponse("automation.runs.list", {
    requestWhere: (payload) => payload.automation_id === automationId,
  });
  await clickOrTap(collaboratorPage.getByTestId("automation-refresh"), mobile);
  const frame = await runs;
  const latestRun = (
    frame.rawPayload as Array<{ id?: string; status?: string; error_message?: string }>
  )?.[0];
  expect(
    latestRun?.status,
    `latest run ended before the Stop check: ${JSON.stringify(latestRun)}`,
  ).toMatch(/^(triggered|task_created)$/);
  await expect(collaboratorPage.getByTestId("automation-stop-run")).toBeVisible({
    timeout: 10_000,
  });
  expect(latestRun?.id).toBeTruthy();
  return latestRun!.id!;
}

async function verifyMemberCannotControlRun(
  state: CredentialDraftState,
  runId: string,
): Promise<void> {
  const memberPage = await state.memberContext.newPage();
  await memberPage.goto(`/automations/${state.automationId}?run=${runId}`);
  if (state.mobile) {
    const runsTrigger = memberPage.getByTestId("runs-drawer-trigger");
    await expect(runsTrigger).toBeVisible({ timeout: 20_000 });
    await runsTrigger.tap();
    const drawer = memberPage.getByTestId("runs-drawer");
    await expect(drawer).toBeVisible();
    const safeRunRow = drawer.getByTestId(`run-row-${runId}`);
    await expect(safeRunRow).toContainText("Running");
    await safeRunRow.scrollIntoViewIfNeeded();
    await expectInViewport(memberPage, safeRunRow);
    await capture(memberPage, state.testInfo, "phone-member-safe-run-status.png");
  } else {
    await expect(memberPage.getByTestId("runs-rail")).toBeVisible({ timeout: 20_000 });
  }
  await expect(memberPage.getByTestId("automation-run-now")).toHaveCount(0);
  await expect(memberPage.getByTestId("automation-stop-run")).toHaveCount(0);
  await memberPage.close();
}

async function stopCredentialRunAndRevoke(
  state: CredentialOwnerState,
  runId: string,
): Promise<void> {
  const { ownerPage, ownerWs, collaboratorPage, mobile, workspaceId, automationId } = state;
  await ownerPage.goto(`/automations/${automationId}?run=${runId}`);
  const runs = ownerWs.waitForResponse("automation.runs.list", {
    requestWhere: (payload) => payload.automation_id === automationId,
  });
  await clickOrTap(ownerPage.getByTestId("automation-refresh"), mobile);
  const frame = await runs;
  const latestRun = (
    frame.rawPayload as Array<{ status?: string; error_message?: string; can_stop?: boolean }>
  )?.[0];
  expect(
    latestRun?.status,
    `owner observed a terminal run before Stop: ${JSON.stringify(latestRun)}`,
  ).toMatch(/^(triggered|task_created)$/);
  expect(latestRun?.can_stop, `owner run projection: ${JSON.stringify(latestRun)}`).toBe(true);
  const stopButton = ownerPage.getByTestId("automation-stop-run");
  await expect(stopButton).toBeVisible({ timeout: 30_000 });
  if (mobile) await assertTouchTarget(stopButton);
  await capture(ownerPage, state.testInfo, `${mobile ? "phone" : "desktop"}-owner-run-stop.png`);
  const stop = ownerWs.waitForResponse("automation.run.stop");
  await clickOrTap(stopButton, mobile);
  await stop;
  await expect(stopButton).toHaveCount(0, { timeout: 15_000 });

  await ownerPage.goto(`/settings/workspaces/${workspaceId}/automations/${automationId}`);
  await expect(ownerPage.getByTestId("automation-editor")).toBeVisible();
  const revokeButton = ownerPage.getByTestId(
    `automation-workspace-credential-revoke-${REPOSITORY_SECRET_KEY}`,
  );
  const revoke = ownerWs.waitForResponse("automation.credential.revoke_workspace");
  await clickOrTap(revokeButton, mobile);
  await revoke;
  await expect(
    ownerPage.getByTestId(`automation-workspace-credential-${REPOSITORY_SECRET_KEY}`),
  ).toContainText("Grant revoked");
  await collaboratorPage.goto(`/settings/workspaces/${workspaceId}/automations/${automationId}`);
  await expect(collaboratorPage.getByTestId("automation-editor")).toBeVisible();
  await expect(
    collaboratorPage.getByTestId(`automation-workspace-credential-${REPOSITORY_SECRET_KEY}`),
  ).toContainText("Grant revoked");
}

async function verifyNoCredentialPath(state: CredentialDraftState): Promise<void> {
  const { collaboratorPage, collaboratorWs, mobile, noCredentialProfileName } = state;
  await collaboratorPage.goto(`/settings/workspaces/${state.workspaceId}/automations/new`);
  await expect(collaboratorPage.getByTestId("automation-editor")).toBeVisible();
  await selectOption(collaboratorPage, "agent-profile-selector", noCredentialProfileName, mobile);
  await expect(collaboratorPage.getByTestId("automation-personal-credential-fields")).toHaveCount(
    0,
  );
  await expect(collaboratorPage.getByTestId("automation-credential-error")).toHaveCount(0);
  const name = "No secret automation path";
  await collaboratorPage.getByTestId("automation-name-input").fill(name);
  const save = collaboratorWs.waitForResponse("automation.create");
  await clickOrTap(state.saveButton, mobile);
  await save;
  await expect(collaboratorPage).toHaveURL(/\/automations$/, { timeout: 15_000 });
  const automationRow = collaboratorPage.getByTestId(/^automation-row-/).filter({ hasText: name });
  await expect(automationRow).toBeVisible();
  const automationId = ((await automationRow.getAttribute("data-testid")) ?? "").replace(
    "automation-row-",
    "",
  );
  expect(automationId).toMatch(/^[0-9a-f-]{36}$/i);
  await collaboratorPage.goto(`/automations/${automationId}`);
  await startCredentialRun({ ...state, automationId });
  if (mobile) {
    await expect
      .poll(() =>
        collaboratorPage.evaluate(() => document.documentElement.scrollWidth <= window.innerWidth),
      )
      .toBe(true);
  }
}

export function registerAutomationCredentialTests(mobile: boolean) {
  let ownerContext: BrowserContext;
  let collaboratorContext: BrowserContext;
  let memberContext: BrowserContext;
  let collaboratorId: string;
  let memberId: string;
  let workspaceId: string;
  let baseUrl: string;
  let interlock: string;
  let agentProfileId: string;
  let agentProfileName: string;
  let noCredentialProfileName: string;
  let workspaceSecretId: string;

  test.describe.serial(`automation credentials ${mobile ? "phone" : "desktop"}`, () => {
    test.beforeAll(async ({ backend, browser }) => {
      await backend.restart({
        KANDEV_FEATURES_AUTH: "true",
        KANDEV_DATABASE_PATH: path.join(backend.tmpDir, "automation-credentials-auth.db"),
      });
      baseUrl = backend.baseUrl;
      interlock = await loadInterimSettingsInterlockToken(baseUrl);
      const device = mobile ? devices["Pixel 5"] : { viewport: { width: 1440, height: 960 } };
      ownerContext = await browser.newContext({ ...device, baseURL: backend.frontendUrl });
      collaboratorContext = await browser.newContext({ ...device, baseURL: backend.frontendUrl });
      memberContext = await browser.newContext({ ...device, baseURL: backend.frontendUrl });

      await setupAdmin(ownerContext, baseUrl, OWNER);
      await login(ownerContext, baseUrl, OWNER);
      const ownerId = await userId(ownerContext, baseUrl);
      await acceptInvite(
        collaboratorContext,
        baseUrl,
        await createInviteToken(ownerContext, baseUrl),
        COLLABORATOR,
      );
      await login(collaboratorContext, baseUrl, COLLABORATOR);
      collaboratorId = await userId(collaboratorContext, baseUrl);
      await acceptInvite(
        memberContext,
        baseUrl,
        await createInviteToken(ownerContext, baseUrl),
        MEMBER,
      );
      await login(memberContext, baseUrl, MEMBER);
      memberId = await userId(memberContext, baseUrl);
      expect(new Set([ownerId, collaboratorId, memberId]).size).toBe(3);

      const workspaceResponse = await ownerContext.request.post(`${baseUrl}/api/v1/workspaces`, {
        headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
        data: { name: "Automation credential workspace" },
      });
      expect(workspaceResponse.ok(), await workspaceResponse.text()).toBe(true);
      const workspace = (await workspaceResponse.json()) as { id: string; owner_id: string };
      expect(workspace.owner_id).toBe(ownerId);
      workspaceId = workspace.id;
      const membership = await ownerContext.request.put(
        `${baseUrl}/api/v1/workspaces/${workspaceId}/members/${collaboratorId}`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: { role: "collaborator" },
        },
      );
      expect(membership.ok(), await membership.text()).toBe(true);
      const memberMembership = await ownerContext.request.put(
        `${baseUrl}/api/v1/workspaces/${workspaceId}/members/${memberId}`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: { role: "collaborator" },
        },
      );
      expect(memberMembership.ok(), await memberMembership.text()).toBe(true);

      const ownerSecret = await createSecret({
        context: ownerContext,
        baseUrl,
        interlock,
        name: OWNER_GLOBAL_SECRET,
        scope: "global",
      });
      const workspaceSecret = await createSecret({
        context: ownerContext,
        baseUrl,
        interlock,
        name: WORKSPACE_SECRET,
        scope: "workspace",
        workspaceId,
      });
      workspaceSecretId = workspaceSecret.id;

      const agentsResponse = await ownerContext.request.get(`${baseUrl}/api/v1/agents`);
      expect(agentsResponse.ok(), await agentsResponse.text()).toBe(true);
      const { agents } = (await agentsResponse.json()) as { agents: Agent[] };
      const mockAgent = agents.find((agent) => agent.name === "mock-agent");
      expect(
        mockAgent?.profiles.length,
        "the isolated E2E backend must expose mock-agent",
      ).toBeGreaterThan(0);
      const sharedProfile = mockAgent!.profiles[0];
      agentProfileId = sharedProfile.id;
      agentProfileName = sharedProfile.name;
      const profileUpdate = await ownerContext.request.patch(
        `${baseUrl}/api/v1/agent-profiles/${agentProfileId}`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: {
            env_vars: [{ key: AGENT_SECRET_KEY, secret_id: ownerSecret.id }],
          },
        },
      );
      expect(profileUpdate.ok(), await profileUpdate.text()).toBe(true);

      noCredentialProfileName = "Automation No Credential Profile";
      const plainProfile = await ownerContext.request.post(
        `${baseUrl}/api/v1/agents/${mockAgent!.id}/profiles`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: {
            name: noCredentialProfileName,
            model: "mock-fast",
            auto_approve: true,
          },
        },
      );
      expect(plainProfile.ok(), await plainProfile.text()).toBe(true);

      const executorsResponse = await ownerContext.request.get(`${baseUrl}/api/v1/executors`);
      expect(executorsResponse.ok(), await executorsResponse.text()).toBe(true);
      const { executors } = (await executorsResponse.json()) as { executors: Executor[] };
      const localExecutor = executors.find((executor) => executor.type === "local");
      expect(localExecutor).toBeDefined();
      const executorProfile = await ownerContext.request.post(
        `${baseUrl}/api/v1/executors/${localExecutor!.id}/profiles`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: {
            name: EXECUTOR_PROFILE_NAME,
            env_vars: [{ key: EXECUTOR_SECRET_KEY, secret_id: ownerSecret.id }],
          },
        },
      );
      expect(executorProfile.ok(), await executorProfile.text()).toBe(true);
      const executorProfileId = ((await executorProfile.json()) as { id: string }).id;
      expect(executorProfileId).toBeTruthy();

      const repositoryPath = path.join(backend.tmpDir, "automation-credential-repo");
      await fs.promises.mkdir(repositoryPath, { recursive: true });
      execFileSync("git", ["init", "--initial-branch=main"], { cwd: repositoryPath });
      execFileSync("git", ["config", "user.email", "automation-e2e@example.dev"], {
        cwd: repositoryPath,
      });
      execFileSync("git", ["config", "user.name", "Automation E2E"], { cwd: repositoryPath });
      await fs.promises.writeFile(
        path.join(repositoryPath, "README.md"),
        "automation credentials\n",
      );
      execFileSync("git", ["add", "README.md"], { cwd: repositoryPath });
      execFileSync("git", ["commit", "-m", "Add automation fixture"], { cwd: repositoryPath });
      const repositoryResponse = await ownerContext.request.post(
        `${baseUrl}/api/v1/workspaces/${workspaceId}/repositories`,
        {
          headers: { "X-Kandev-Interim-Settings-Interlock": interlock },
          data: {
            name: REPOSITORY_NAME,
            source_type: "local",
            local_path: repositoryPath,
            default_branch: "main",
            secret_bindings: [{ key: REPOSITORY_SECRET_KEY, secret_id: workspaceSecretId }],
          },
        },
      );
      expect(repositoryResponse.ok(), await repositoryResponse.text()).toBe(true);
    });

    test.afterAll(async ({ backend }) => {
      await Promise.all([
        ownerContext?.close(),
        collaboratorContext?.close(),
        memberContext?.close(),
      ]);
      await backend.restart();
    });

    test("validates, binds, grants, runs, and stops a credential-backed automation", async ({
      backend: testBackend,
    }, testInfo) => {
      test.setTimeout(240_000);
      expect(testBackend.baseUrl).toBe(baseUrl);
      const base: CredentialTestBase = {
        mobile,
        testInfo,
        ownerContext,
        collaboratorContext,
        memberContext,
        workspaceId,
        baseUrl,
        interlock,
        agentProfileName,
        noCredentialProfileName,
        workspaceSecretId,
      };
      const draft = await createCredentialDraft(base);
      await bindPersonalSecrets(draft);
      const approved = await grantWorkspaceSecret(draft);
      await enableCredentialAutomation(draft);
      const runId = await startCredentialRun(draft);
      await verifyMemberCannotControlRun(draft, runId);
      await stopCredentialRunAndRevoke(approved, runId);
      await verifyNoCredentialPath(draft);
      await Promise.all([approved.ownerPage.close(), draft.collaboratorPage.close()]);
    });
  });
}
