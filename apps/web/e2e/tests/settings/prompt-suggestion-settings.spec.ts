import { expect, test } from "../../fixtures/test-base";

const PATH = "/settings/preferences/task-behavior?tab=conversation";
const AGENT_ID = "builtin-suggest-next-prompt";

// @covers AC-UI-PROMPT-SUGGEST-001.1 AC-UI-PROMPT-SUGGEST-001.3 AC-UI-PROMPT-SUGGEST-001.5 AC-UI-PROMPT-SUGGEST-001.6
test("prompt suggestion switches reveal the fallback profile and persist", async ({
  testPage,
  apiClient,
  seedData,
}) => {
  const { agents } = await apiClient.listAgents();
  const profile = agents
    .flatMap((agent) => agent.profiles ?? [])
    .find((candidate) => candidate.id === seedData.agentProfileId);
  if (!profile) throw new Error(`seed profile ${seedData.agentProfileId} was not found`);
  const profileLabel = `${profile.agentDisplayName} • ${profile.name}`;
  const baselineResponse = await apiClient.rawRequest("GET", `/api/v1/utility/agents/${AGENT_ID}`);
  expect(baselineResponse.ok).toBe(true);
  const baseline = await baselineResponse.json();

  try {
    await testPage.goto(PATH);
    const card = testPage.getByTestId("task-behavior-settings");
    const main = card.locator("#prompt-suggestions");
    await expect(main).toHaveAttribute("aria-checked", "false");
    await expect(card.locator("#prompt-suggestions-fallback")).toHaveCount(0);

    await main.click();
    const fallback = card.locator("#prompt-suggestions-fallback");
    await expect(fallback).toHaveAttribute("aria-checked", "false");
    await expect(card.getByTestId("prompt-suggestion-profile-picker")).toHaveCount(0);

    await fallback.click();
    const picker = card.getByTestId("prompt-suggestion-profile-picker");
    await expect(picker).toBeVisible();

    // The profile guidance lives in the row's info popover, like other settings.
    await card.getByRole("button", { name: "About Agent profile" }).hover();
    await expect(testPage.getByRole("tooltip")).toContainText("fast");
    await expect(testPage.getByRole("tooltip")).toContainText("up to 15 seconds");
    await testPage.mouse.move(0, 0, { steps: 5 });
    await picker.click();
    await testPage
      .getByRole("listbox")
      .locator(`[data-value="${seedData.agentProfileId}"]`)
      .click();
    await expect(picker).toContainText(profileLabel);

    const floatingSave = testPage.getByTestId("settings-floating-save");
    await floatingSave.getByRole("button", { name: "Save changes" }).click();
    await expect(floatingSave).not.toBeVisible({ timeout: 10_000 });

    await expect
      .poll(async () => {
        const response = await apiClient.rawRequest("GET", "/api/v1/user/settings");
        const body = await response.json();
        return [body.settings.prompt_suggestions, body.settings.prompt_suggestions_fallback];
      })
      .toEqual([true, true]);
    const persisted = await (
      await apiClient.rawRequest("GET", `/api/v1/utility/agents/${AGENT_ID}`)
    ).json();
    expect(persisted).toMatchObject({
      agent_profile_id: seedData.agentProfileId,
      profile_binding_state: "explicit",
    });

    await testPage.reload();
    await expect(card.locator("#prompt-suggestions")).toHaveAttribute("aria-checked", "true");
    await expect(card.locator("#prompt-suggestions-fallback")).toHaveAttribute(
      "aria-checked",
      "true",
    );
    await expect(card.getByTestId("prompt-suggestion-profile-picker")).toContainText(profileLabel);

    await testPage.goto("/settings/utility-agents");
    await expect(testPage.getByTestId(`utility-profile-picker-action-${AGENT_ID}`)).toContainText(
      profileLabel,
    );
  } finally {
    await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
      prompt_suggestions: false,
      prompt_suggestions_fallback: false,
    });
    await apiClient.rawRequest("PATCH", `/api/v1/utility/agents/${AGENT_ID}`, {
      agent_profile_id: baseline.agent_profile_id,
      profile_binding_state: baseline.profile_binding_state,
      enabled: baseline.enabled,
    });
  }
});
