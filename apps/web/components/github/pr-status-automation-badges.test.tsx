import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { i18n } from "@/lib/i18n";
import type { TaskCIAutomationOptions, TaskPR } from "@/lib/types/github";
import {
  AutomationFlagBadges,
  automationAriaSuffix,
  automationForPR,
  automationForPRs,
} from "./pr-status-automation-badges";
import type { AutomationFlags } from "./pr-status-automation-badges";

const PR_EVENTS_BADGE_TESTID = "pr-status-pr-events-chip";
const PR = { repository_id: "repo-1", pr_number: 42 } as TaskPR;

function makeOptions(overrides: Partial<TaskCIAutomationOptions> = {}): TaskCIAutomationOptions {
  return {
    task_id: "task-1",
    auto_fix_enabled: false,
    auto_merge_enabled: false,
    auto_fix_prompt_override: null,
    effective_auto_fix_prompt: "Default CI fix prompt",
    using_default_prompt: true,
    updated_at: "2026-08-10T10:00:00Z",
    pr_states: [],
    ...overrides,
  };
}

function flags(overrides: Partial<AutomationFlags> = {}): AutomationFlags {
  return {
    autoFix: false,
    autoMerge: false,
    autoFixRound: null,
    promptOnReviewRequested: false,
    promptOnMerged: false,
    promptOnClosed: false,
    ...overrides,
  };
}

afterEach(cleanup);

describe("AutomationFlagBadges", () => {
  it.each([
    [flags(), null, null],
    [flags({ promptOnReviewRequested: true }), "PR events 1/3", "1"],
    [flags({ promptOnReviewRequested: true, promptOnMerged: true }), "PR events 2/3", "2"],
    [
      flags({ promptOnReviewRequested: true, promptOnMerged: true, promptOnClosed: true }),
      "PR events 3/3",
      "3",
    ],
  ])(
    "renders one grouped badge for the enabled PR event count",
    (automation, expectedText, expectedCount) => {
      render(<AutomationFlagBadges automation={automation} />);

      const badge = screen.queryByTestId(PR_EVENTS_BADGE_TESTID);
      if (!expectedText) {
        expect(badge).toBeNull();
        return;
      }
      expect(badge?.textContent).toBe(expectedText);
      expect(badge?.getAttribute("data-pr-events-count")).toBe(expectedCount);
      expect(badge?.getAttribute("data-legacy-testid")).toBe("pr-status-follow-up-chip");
    },
  );

  it("derives the same task-wide PR events for single and multiple PR chips", () => {
    const options = makeOptions({
      prompt_on_review_requested: true,
      prompt_on_merged: true,
      prompt_on_closed: true,
    });

    const single = automationForPR(options, PR);
    const multiple = automationForPRs(options, [PR, { ...PR, pr_number: 43 }]);

    expect(multiple).toMatchObject({
      promptOnReviewRequested: single.promptOnReviewRequested,
      promptOnMerged: single.promptOnMerged,
      promptOnClosed: single.promptOnClosed,
    });
    expect(automationAriaSuffix(single, i18n.t)).toBe(
      ", Your review is requested, PR merged, PR closed without merging",
    );
    expect(automationAriaSuffix(multiple, i18n.t)).toBe(
      ", Your review is requested, PR merged, PR closed without merging",
    );
  });
});
