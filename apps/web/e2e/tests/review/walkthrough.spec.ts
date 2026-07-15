import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";
import type { ApiClient } from "../../helpers/api-client";
import type { SeedData } from "../../fixtures/test-base";
import type { Page, Locator } from "@playwright/test";
import { expectWalkthroughBehindDialog } from "./walkthrough-layering";

async function seedWalkthroughTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
  scenario: string,
  doneText: string,
): Promise<SessionPage> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    "Walkthrough E2E",
    seedData.agentProfileId,
    {
      description: `/e2e:${scenario}`,
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );

  await testPage.goto(`/t/${task.id}`);
  const session = new SessionPage(testPage);
  await session.waitForLoad();
  await expect(session.chat.getByText(doneText, { exact: false })).toBeVisible({ timeout: 45_000 });
  await session.waitForChatIdle();
  return session;
}

async function seedWalkthroughChangesTask(
  testPage: Page,
  apiClient: ApiClient,
  seedData: SeedData,
): Promise<SessionPage> {
  return seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-setup", "changes ready");
}

async function customizeChangesWalkthroughPrompt(
  apiClient: ApiClient,
): Promise<() => Promise<void>> {
  const { prompts } = await apiClient.listPrompts();
  const prompt = prompts.find((p) => p.name === "changes-walkthrough");
  expect(prompt, "changes-walkthrough built-in prompt should be seeded").toBeTruthy();
  const originalContent = prompt!.content;
  await apiClient.updatePrompt(prompt!.id, {
    content: [
      "E2E_CUSTOM_CHANGES_WALKTHROUGH",
      "Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.",
      "",
      "Walkthrough requirements:",
      "- Inspect the changed files yourself instead of relying on UI-provided paths.",
      "- For PR tasks, compare the PR head against the PR base branch.",
      "- For non-PR tasks, compare against the task or repository base.",
      "- The first walkthrough step must contextualize the whole change.",
      "- Anchor the first step to the most representative changed line or range.",
      "- Include an `ELI5:` line in the first step text.",
      "- Use `line_end` whenever a logical explanation spans multiple lines.",
      "- For PR-only files, do not assume the PR head is checked out locally.",
      "- Keep each step concise and direct. Do not include a `Justification:` preamble.",
    ].join("\n"),
  });
  return () => apiClient.updatePrompt(prompt!.id, { content: originalContent });
}

/** Open the floating walkthrough card via the launcher pill. */
async function openWalkthrough(testPage: Page): Promise<Locator> {
  const launcher = testPage.getByTestId("walkthrough-launcher");
  await expect(launcher).toBeVisible({ timeout: 30_000 });
  await launcher.click();
  const card = testPage.getByTestId("walkthrough-floating");
  await expect(card).toBeVisible({ timeout: 30_000 });
  return card;
}

test.describe("Code walkthrough", () => {
  test.describe.configure({ retries: 2, timeout: 120_000 });

  test("Changes-panel request asks the agent to walk through current changes", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedWalkthroughChangesTask(testPage, apiClient, seedData);
    const restorePrompt = await customizeChangesWalkthroughPrompt(apiClient);
    try {
      await expect(session.walkthroughLauncher()).toHaveCount(0);
      await session.clickTab("Changes");
      const request = session.changesRequestWalkthroughButton();
      await expect(request).toBeVisible({ timeout: 15_000 });
      await expect(request).toContainText("Walkthrough");
      await expect(request).toBeEnabled({ timeout: 30_000 });

      await request.click();
      await session.showSessionContext();

      await expect(session.activeChat()).toContainText("@changes-walkthrough", {
        timeout: 15_000,
      });
      await expect(session.activeChat()).not.toContainText("Diff context:");
      await expect(session.activeChat()).not.toContainText("Base branch:");
      await expect(session.activeChat()).not.toContainText("Changed files:");
      await expect(session.activeChat()).not.toContainText("walkthrough_a.txt [");
      await expect(session.activeChat()).not.toContainText("E2E_CUSTOM_CHANGES_WALKTHROUGH");
      await expect(session.activeChat()).toContainText("Walkthrough: Tour of the change", {
        timeout: 45_000,
      });
      await expect(session.activeChat().locator(".tabler-icon-route")).toBeVisible();
      await expect(session.activeChat()).toContainText("walkthrough-request complete", {
        timeout: 45_000,
      });

      const card = await openWalkthrough(testPage);
      await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 1 / 5");
      await expect(card.getByTestId("walkthrough-step-title")).toContainText("Overview");
      await expect(card.getByTestId("walkthrough-step-body")).toContainText("ELI5:");
      await expect(session.walkthroughEditorRange()).toBeVisible({ timeout: 15_000 });
    } finally {
      await restorePrompt();
    }
  });

  test("expanded review toolbar sends the saved walkthrough prompt reference", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedWalkthroughChangesTask(testPage, apiClient, seedData);
    const restorePrompt = await customizeChangesWalkthroughPrompt(apiClient);
    try {
      await session.clickTab("Changes");

      await session.changes.getByRole("button", { name: "Review" }).click();
      const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
      await expect(dialog).toBeVisible({ timeout: 15_000 });

      const request = session.reviewRequestWalkthroughButton();
      await expect(request).toBeVisible();
      await expect(request).toBeEnabled({ timeout: 30_000 });
      await expect(request).toHaveAttribute("aria-label", "Walk me through these review changes");
      await request.click();

      await dialog.getByRole("button", { name: "Close review" }).click();
      await expect(dialog).toBeHidden({ timeout: 15_000 });
      await session.showSessionContext();

      await expect(session.activeChat()).toContainText("@changes-walkthrough", {
        timeout: 15_000,
      });
      await expect(session.activeChat()).not.toContainText("Diff context:");
      await expect(session.activeChat()).not.toContainText("Base branch:");
      await expect(session.activeChat()).not.toContainText("Changed files:");
      await expect(session.activeChat()).not.toContainText("walkthrough_a.txt [");
      await expect(session.activeChat()).not.toContainText("E2E_CUSTOM_CHANGES_WALKTHROUGH");
      await expect(session.activeChat()).toContainText("walkthrough-request complete", {
        timeout: 45_000,
      });
    } finally {
      await restorePrompt();
    }
  });

  test("floating card walks all steps across changed and unchanged files", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);
    const header = card.getByTestId("walkthrough-step-header");
    const body = card.getByTestId("walkthrough-step-body");

    await expect(header).toContainText("Step 1 / 5");
    await expect(card.getByTestId("walkthrough-step-title")).toContainText("Overview");
    await expect(body).toContainText("ELI5:");
    await expect(card.getByTestId("walkthrough-step-file")).toContainText("walkthrough_a.txt");
    await expect(card.getByTestId("walkthrough-prev")).toBeDisabled();

    const expectStep = async (n: number, text: string) => {
      await card.getByTestId("walkthrough-next").click();
      await expect(header).toContainText(`Step ${n} / 5`);
      await expect(body).toContainText(text);
    };
    await expectStep(2, "WALKTHROUGH_CHANGE_A");
    await expectStep(3, "WALKTHROUGH_CHANGE_B");
    await expectStep(4, "WALKTHROUGH_CHANGE_C");
    await expectStep(5, "WALKTHROUGH_UNCHANGED");
    await expect(card.getByTestId("walkthrough-next")).toBeDisabled();

    // The unchanged/base file is opened in an editor tab, but it does not belong
    // in the Review diff because it was not changed by this task.
    await expect(testPage.locator('.dv-default-tab:has-text("walkthrough_base.txt")')).toBeVisible({
      timeout: 15_000,
    });
    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const dialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(dialog).toBeVisible({ timeout: 15_000 });
    await expect(
      dialog.getByTestId("review-dialog-sidebar").getByText("walkthrough_base.txt"),
    ).toHaveCount(0);
    await expect(
      dialog.getByTestId("changes-repo-group").getByText("walkthrough_base.txt"),
    ).toHaveCount(0);
  });

  test("editor walkthrough shows range marker, connector, and supports dragging", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    await expect(testPage.getByTestId("walkthrough-editor-range")).toBeVisible({
      timeout: 15_000,
    });
    await expect(testPage.getByTestId("walkthrough-connector")).toBeVisible({ timeout: 15_000 });

    await card.getByTestId("walkthrough-next").click();
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 2 / 5");
    await expect(testPage.getByTestId("walkthrough-editor-range")).toHaveAttribute(
      "data-line-range",
      "2-3",
      { timeout: 15_000 },
    );

    const before = await card.boundingBox();
    if (!before) throw new Error("walkthrough card missing before drag");
    const dragHandle = card.getByTestId("walkthrough-drag-handle");
    const handleBox = await dragHandle.boundingBox();
    if (!handleBox) throw new Error("walkthrough drag handle missing");
    await testPage.mouse.move(
      handleBox.x + handleBox.width / 2,
      handleBox.y + handleBox.height / 2,
    );
    await testPage.mouse.down();
    await testPage.mouse.move(handleBox.x - 120, handleBox.y + 80, { steps: 6 });
    await testPage.mouse.up();

    const after = await card.boundingBox();
    if (!after) throw new Error("walkthrough card missing after drag");
    expect(Math.abs(after.x - before.x)).toBeGreaterThan(40);
    expect(Math.abs(after.y - before.y)).toBeGreaterThan(30);
  });

  test("step file label is shown and opens the file", async ({ testPage, apiClient, seedData }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    const fileLabel = card.getByTestId("walkthrough-step-file");
    await expect(fileLabel).toContainText("walkthrough_a.txt");
    await fileLabel.click();
    await expect(testPage.locator('.dv-default-tab:has-text("walkthrough_a.txt")')).toBeVisible({
      timeout: 15_000,
    });
  });

  test("ask box offers Add (queue) and Run (ask now)", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);

    await card.getByRole("textbox").fill("Why does this line exist?");
    await expect(card.getByRole("button", { name: "Add" })).toBeEnabled();
    await expect(card.getByRole("button", { name: "Run" })).toBeEnabled();
    await card.getByRole("button", { name: "Run" }).click();
    await expect(card.getByRole("textbox")).toHaveValue("");
  });

  test("closing minimizes the card but keeps the launcher; reopen restores it", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    await seedWalkthroughTask(testPage, apiClient, seedData, "walkthrough-basic", "5-step tour");
    const card = await openWalkthrough(testPage);
    const range = testPage.getByTestId("walkthrough-editor-range");
    await expect(range).toBeVisible({ timeout: 15_000 });

    await card.getByTestId("walkthrough-close").click();
    await expect(card).toBeHidden({ timeout: 5_000 });
    await expect(range).toHaveCount(0);
    // The launcher persists — the tour is not lost, just minimized.
    const launcher = testPage.getByTestId("walkthrough-launcher");
    await expect(launcher).toBeVisible();
    await launcher.click();
    await expect(testPage.getByTestId("walkthrough-floating")).toBeVisible({ timeout: 10_000 });
    await expect(range).toBeVisible({ timeout: 15_000 });
  });

  test("discard removes the persisted walkthrough after confirmation", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const session = await seedWalkthroughTask(
      testPage,
      apiClient,
      seedData,
      "walkthrough-basic",
      "5-step tour",
    );
    const card = await openWalkthrough(testPage);
    await expect(session.walkthroughEditorRange()).toBeVisible({ timeout: 15_000 });

    await testPage.evaluate(() => window.dispatchEvent(new CustomEvent("open-review-dialog")));
    const reviewDialog = testPage.getByRole("dialog", { name: "Review Changes" });
    await expect(reviewDialog).toBeVisible({ timeout: 15_000 });
    await expectWalkthroughBehindDialog(testPage, reviewDialog, [
      { locator: card, name: "walkthrough window" },
      { locator: session.walkthroughLauncher().locator(".."), name: "walkthrough launcher" },
    ]);
    await reviewDialog.getByRole("button", { name: "Close review" }).click();
    await expect(reviewDialog).toBeHidden({ timeout: 15_000 });

    await session.walkthroughLauncher().hover();
    await expect(session.walkthroughDiscardButton()).toBeVisible({ timeout: 5_000 });
    await session.walkthroughDiscardButton().click();
    const discardDialog = session.walkthroughDiscardDialog();
    await expect(discardDialog).toBeVisible();
    await expectWalkthroughBehindDialog(testPage, discardDialog, [
      { locator: card, name: "walkthrough window" },
      { locator: session.walkthroughLauncher().locator(".."), name: "walkthrough launcher" },
    ]);
    await discardDialog.getByRole("button", { name: "Cancel" }).click();
    await expect(card).toBeVisible();
    await expect(session.walkthroughLauncher()).toHaveCount(1);

    await session.walkthroughLauncher().hover();
    await session.walkthroughDiscardButton().click();
    await expect(session.walkthroughDiscardDialog()).toBeVisible();
    await session
      .walkthroughDiscardDialog()
      .getByRole("button", { name: "Discard walkthrough" })
      .click();

    await expect(session.walkthroughDiscardDialog()).toBeHidden({ timeout: 10_000 });
    await expect(session.walkthroughLauncher()).toHaveCount(0);
    await expect(session.walkthroughFloating()).toHaveCount(0);
    await expect(session.walkthroughEditorRange()).toHaveCount(0);
  });

  test("a re-emitted walkthrough replaces the previous one without a page reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    // The agent emits a 2-step tour, then a different 3-step tour. Opening the
    // card refetches the latest, so the re-emit shows without reloading.
    await seedWalkthroughTask(
      testPage,
      apiClient,
      seedData,
      "walkthrough-reemit",
      "reemit-second-done",
    );
    const card = await openWalkthrough(testPage);

    await expect(testPage.getByTestId("walkthrough-launcher")).toHaveCount(1);
    await expect(card.getByTestId("walkthrough-step-header")).toContainText("Step 1 / 3");
    await expect(card.getByTestId("walkthrough-step-body")).toContainText("REEMIT_SECOND");
    await expect(card.getByTestId("walkthrough-step-body")).not.toContainText("REEMIT_FIRST");
  });
});
