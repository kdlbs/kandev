import { test, expect } from "../../fixtures/test-base";
import { SessionPage } from "../../pages/session-page";

/**
 * Verifies the chat-input model selector's failure path:
 * when the backend rejects a model change, the UI shows a toast carrying the
 * backend error message and the trigger label reverts to the previous model.
 *
 * mock-agent exposes "model" as a config option, so changing the model goes
 * through POST /set-config-option. Agents without a model config option use
 * POST /set-model — both routes are stubbed for resilience.
 */
test.describe("Chat model selector — RPC failure", () => {
  test("shows error toast and reverts trigger label when model change fails", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Error Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const baseline = sessions[0]?.metadata?.acp_config_baseline as
          | Record<string, string>
          | undefined;
        return baseline?.effort;
      })
      .toBe("medium");

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toBeVisible({ timeout: 15_000 });
    // Baseline-matching secondary options stay out of the task-chat trigger.
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    await trigger.hover();
    const tooltip = testPage.getByRole("tooltip");
    await expect(tooltip).toContainText("Model: Mock Fast");
    await expect(tooltip).toContainText("Effort: Medium");
    await expect(tooltip).not.toContainText("Controls how much reasoning the mock model uses");
    await testPage.mouse.move(0, 0);

    const backendErrorMessage = "mock backend rejected model change";
    const fail = (route: import("@playwright/test").Route) =>
      route.fulfill({
        status: 500,
        contentType: "application/json",
        body: JSON.stringify({ error: backendErrorMessage }),
      });
    await testPage.route("**/set-model", fail);
    await testPage.route("**/set-config-option", fail);

    await trigger.click();
    const smartRow = testPage.getByRole("option", { name: /Mock Smart/ });
    await expect(smartRow).toBeVisible({ timeout: 5_000 });
    await smartRow.click();

    const toast = testPage
      .getByTestId("toast-message")
      .filter({ hasText: "Failed to change model" });
    await expect(toast).toBeVisible({ timeout: 5_000 });
    await expect(toast).toContainText(backendErrorMessage);

    // After revert the trigger label should match the original (model + extras).
    await expect(trigger).toContainText("Mock Fast", { timeout: 5_000 });
    await expect(trigger).not.toContainText("Mock Smart");
  });

  test("stale failure does not revert a newer successful change", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Race Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    // Hold the first config-option request open so we can fire a second one
    // before the first settles. Resolve the first as a 500 after the second
    // has already been issued — the stale rejection must NOT clobber the
    // newer optimistic state.
    let releaseFirst: (() => void) | null = null;
    const firstHeld = new Promise<void>((resolve) => {
      releaseFirst = resolve;
    });
    // Resolves once the first (stale) request has fully completed its
    // route.fulfill, so the test can deterministically await propagation
    // instead of using a fixed sleep.
    let firstSettled: (() => void) | null = null;
    const firstSettledPromise = new Promise<void>((resolve) => {
      firstSettled = resolve;
    });
    let callCount = 0;
    await testPage.route("**/set-config-option", async (route) => {
      callCount += 1;
      if (callCount === 1) {
        await firstHeld;
        await route.fulfill({
          status: 500,
          contentType: "application/json",
          body: JSON.stringify({ error: "stale failure" }),
        });
        firstSettled?.();
        return;
      }
      return route.fulfill({
        status: 200,
        contentType: "application/json",
        body: JSON.stringify({ ok: true }),
      });
    });

    await trigger.click();
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    await expect(trigger).toContainText("Mock Smart", { timeout: 5_000 });
    // The popover stays open because mock-agent also exposes extra config
    // options. Pick again — second request will succeed. mock-agent only ships
    // two models (Mock Fast / Mock Smart), so we go back to Mock Fast.
    await testPage.getByRole("option", { name: /Mock Fast/ }).click();

    // Now release the first (stale) request — its 500 rejection should be
    // swallowed (no toast).
    releaseFirst?.();
    await firstSettledPromise;
    await expect(testPage.getByTestId("toast-message")).toHaveCount(0);
    // Trigger should still reflect the newer (successful) selection.
    await expect(trigger).toContainText("Mock Fast", { timeout: 5_000 });
  });
});

/**
 * Verifies that a model change selected via the chat-input model selector
 * survives a full page reload. The regression this guards against: the
 * SetConfigOption RPC path used by mock-agent did not emit the session_models
 * convergence event, so the orchestrator never persisted the new model to the
 * session snapshot and SSR re-served the pre-change model on reload.
 */
test.describe("Chat model selector — persistence", () => {
  test("completed turn metadata keeps changed options after a page reload", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Turn Config Reload Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );
    if (!task.session_id) throw new Error("expected an auto-started session");

    await testPage.goto(`/t/${task.id}`);
    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await trigger.click();
    await testPage.getByTestId("config-option-trigger-effort").click();
    await testPage.getByRole("button", { name: "High", exact: true }).click();
    await expect(trigger).toHaveText("Mock Fast / High", { timeout: 5_000 });
    await testPage.keyboard.press("Escape");

    const prompt = "second turn uses high effort";
    await session.sendMessage(prompt);
    await session.expectChatResponseVisible(prompt);
    await session.waitForChatIdle({ timeout: 30_000 });

    const capturedConfig = testPage.getByText("mock-fast · Effort: High", { exact: true });
    await expect(capturedConfig.first()).toHaveText("mock-fast · Effort: High");

    await testPage.reload();
    await session.waitForLoad();

    const messagesResponse = await testPage.request.get(
      `/api/v1/task-sessions/${task.session_id}/messages?limit=50&sort=desc`,
    );
    const messagesPayload = (await messagesResponse.json()) as {
      messages: { content: string; turn_id?: string }[];
    };
    const responseMessage = messagesPayload.messages.find((message) =>
      message.content.includes(prompt),
    );
    expect(responseMessage?.turn_id).toBeTruthy();

    const turnsResponse = await testPage.request.get(
      `/api/v1/task-sessions/${task.session_id}/turns`,
    );
    const turnsPayload = (await turnsResponse.json()) as {
      turns: { id: string; metadata?: Record<string, unknown> }[];
    };
    const responseTurn = turnsPayload.turns.find((turn) => turn.id === responseMessage?.turn_id);
    expect(responseTurn?.metadata).toMatchObject({
      runtime_config_snapshot: {
        config_options: expect.arrayContaining([
          expect.objectContaining({ id: "effort", value: "high" }),
        ]),
        config_baseline: { effort: "medium" },
      },
    });

    await expect(capturedConfig.first()).toHaveText("mock-fast · Effort: High");
  });

  test("profile config overrides provider defaults on the first rendered label", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const { agents } = await apiClient.listAgents();
    const agent = agents.find((item) => item.name === "mock-agent") ?? agents[0];
    const profile = await apiClient.createAgentProfile(agent.id, "High Effort Task Profile", {
      model: "mock-fast",
      config_options: { effort: "high" },
    });

    try {
      const task = await apiClient.createTaskWithAgent(
        seedData.workspaceId,
        "Profile Model Selector Defaults Test",
        profile.id,
        {
          description: "/e2e:simple-message",
          workflow_id: seedData.workflowId,
          workflow_step_id: seedData.startStepId,
          repository_ids: [seedData.repositoryId],
        },
      );
      await testPage.goto(`/t/${task.id}`);
      const session = new SessionPage(testPage);
      await session.waitForLoad();
      await session.waitForChatIdle({ timeout: 30_000 });

      await expect
        .poll(async () => {
          const { sessions } = await apiClient.listTaskSessions(task.id);
          const metadata = sessions[0]?.metadata;
          const runtime = metadata?.runtime_config as
            | { config_options?: Record<string, string> }
            | undefined;
          const baseline = metadata?.acp_config_baseline as Record<string, string> | undefined;
          return `${runtime?.config_options?.effort}/${baseline?.effort}`;
        })
        .toBe("high/medium");

      const trigger = testPage.getByRole("button", { name: "Session model settings" });
      await expect(trigger).toHaveText("Mock Fast / High", { timeout: 15_000 });

      await testPage.addInitScript(() => {
        const win = window as Window & { __modelSelectorTexts?: string[] };
        const capture = () => {
          const text = document
            .querySelector('button[aria-label="Session model settings"]')
            ?.textContent?.trim();
          if (text && win.__modelSelectorTexts?.at(-1) !== text) {
            win.__modelSelectorTexts?.push(text);
          }
        };
        win.__modelSelectorTexts = [];
        new MutationObserver(capture).observe(document, {
          subtree: true,
          childList: true,
          characterData: true,
        });
        document.addEventListener("DOMContentLoaded", capture);
      });

      await testPage.reload();
      await session.waitForLoad();
      await expect(trigger).toHaveText("Mock Fast / High", { timeout: 15_000 });
      const renderedLabels = await testPage.evaluate(
        () =>
          (window as Window & { __modelSelectorTexts?: string[] }).__modelSelectorTexts?.filter(
            (text) => text.startsWith("Mock Fast"),
          ) ?? [],
      );
      expect(renderedLabels.length).toBeGreaterThan(0);
      expect(renderedLabels).toEqual(renderedLabels.map(() => "Mock Fast / High"));
    } finally {
      await apiClient.deleteAgentProfile(profile.id, true);
    }
  });

  test("changed values stay compact across backend restart", async ({
    testPage,
    apiClient,
    seedData,
    backend,
  }, testInfo) => {
    test.setTimeout(120_000);

    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Persistence Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const baseline = sessions[0]?.metadata?.acp_config_baseline as
          | Record<string, string>
          | undefined;
        return baseline?.effort;
      })
      .toBe("medium");

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toHaveText("Mock Fast", { timeout: 15_000 });

    // Model help is visible in the model list. Option help remains hidden in
    // the compact top-level list until the user enters that option submenu.
    await trigger.click();
    await expect(testPage.getByText("Fast mock model for testing", { exact: true })).toBeVisible();
    await expect(testPage.getByText("Smart mock model for testing", { exact: true })).toBeVisible();
    await expect(
      testPage.getByText("Controls how much reasoning the mock model uses", { exact: true }),
    ).toHaveCount(0);

    // Change both model and effort. The closed task trigger lists every
    // changed value in ACP order while omitting the baseline Medium value.
    // Model changes refresh the provider's full option snapshot, so wait for
    // that convergence before applying a dependent option.
    await testPage.getByRole("option", { name: /Mock Smart/ }).click();
    await expect(trigger).toHaveText("Mock Smart", { timeout: 5_000 });
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const runtime = sessions[0]?.metadata?.runtime_config as { model?: string } | undefined;
        return runtime?.model;
      })
      .toBe("mock-smart");
    await testPage.keyboard.press("Escape");
    await expect(testPage.getByRole("option", { name: /Mock Smart/ })).toBeHidden();
    await trigger.click();
    await testPage.getByTestId("config-option-trigger-effort").click();
    await expect(
      testPage.getByText("Controls how much reasoning the mock model uses", { exact: true }),
    ).toBeVisible();
    await testPage.getByRole("button", { name: "Low", exact: true }).click();
    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const runtime = sessions[0]?.metadata?.runtime_config as
          | { config_options?: Record<string, string> }
          | undefined;
        return runtime?.config_options?.effort;
      })
      .toBe("low");
    await expect(trigger).toHaveText("Mock Smart / Low", { timeout: 5_000 });
    await expect(trigger).not.toContainText("Medium");

    await testInfo.attach("task-model-selector-desktop", {
      body: await testPage.screenshot(),
      contentType: "image/png",
    });

    // A full backend recreation keeps both the mutable values and the original
    // persisted baseline, so the same differences remain visible after resume.
    await backend.restart();
    await testPage.reload();
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 60_000 });

    await expect
      .poll(async () => {
        const { sessions } = await apiClient.listTaskSessions(task.id);
        const metadata = sessions[0]?.metadata;
        const runtime = metadata?.runtime_config as
          | { config_options?: Record<string, string> }
          | undefined;
        const baseline = metadata?.acp_config_baseline as Record<string, string> | undefined;
        return `${runtime?.config_options?.effort}/${baseline?.effort}`;
      })
      .toBe("low/medium");
    await trigger.click();
    await expect(testPage.getByTestId("config-option-trigger-effort")).toContainText("Low");
    await testPage.keyboard.press("Escape");
    await expect(trigger).toHaveText("Mock Smart / Low", { timeout: 15_000 });
    await expect(trigger).not.toContainText("Medium");
  });
});

/**
 * Verifies the chat-input model selector's popover stays open after picking a
 * model when extra config options (reasoning effort, thought level, …) are
 * present in the same selector. Picking a model is rarely the last thing a user
 * wants to do when there are other knobs available — they typically want to
 * adjust effort too. Auto-closing on model select forces a second open.
 *
 * mock-agent ships with model + effort config options, so the popover renders
 * an Effort row below the model list and opens the Effort picker from there.
 */
test.describe("Chat model selector — popover open/close behavior", () => {
  test("stays open after picking a model when extra config options exist", async ({
    testPage,
    apiClient,
    seedData,
  }) => {
    const task = await apiClient.createTaskWithAgent(
      seedData.workspaceId,
      "Model Selector Stay Open Test",
      seedData.agentProfileId,
      {
        description: "/e2e:simple-message",
        workflow_id: seedData.workflowId,
        workflow_step_id: seedData.startStepId,
        repository_ids: [seedData.repositoryId],
      },
    );

    await testPage.goto(`/t/${task.id}`);

    const session = new SessionPage(testPage);
    await session.waitForLoad();
    await session.waitForChatIdle({ timeout: 30_000 });

    const trigger = testPage.getByRole("button", { name: "Session model settings" });
    await expect(trigger).toContainText("Mock Fast", { timeout: 15_000 });

    await trigger.click();
    // The effort config row is the rendered marker that the popover content is
    // mounted. It opens the nested effort selector inside the same popover.
    const effortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(effortTrigger).toBeVisible({ timeout: 5_000 });

    await testPage.getByRole("option", { name: /Mock Smart/ }).click();

    // The trigger label updates optimistically — wait for that so we know the
    // selection round-tripped through the handler.
    await expect(trigger).toContainText("Mock Smart", { timeout: 5_000 });

    // Popover must still be open so the user can also pick an effort level
    // without re-opening. The effort row is rendered only while PopoverContent
    // is mounted (Radix unmounts it on close).
    const updatedEffortTrigger = testPage.getByTestId("config-option-trigger-effort");
    await expect(updatedEffortTrigger).toBeVisible();
  });
});
