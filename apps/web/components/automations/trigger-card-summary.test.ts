import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import type { AutomationTrigger } from "@/lib/types/automation";
import { getTriggerSummary } from "./trigger-card";

function prTrigger(events: string[]): AutomationTrigger {
  return {
    id: "trigger-1",
    automation_id: "automation-1",
    type: "github_pr",
    config: { events },
    enabled: true,
    created_at: "",
    updated_at: "",
    last_evaluated_at: null,
  } as AutomationTrigger;
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-002.7
describe("getTriggerSummary", () => {
  it("names pull request events by their labels, not their saved ids", () => {
    expect(getTriggerSummary(prTrigger(["opened", "new_commits", "review_requested"]), t)).toBe(
      "PR: Opened, New commits, Review requested",
    );
  });

  it("keeps an event id this build does not know", () => {
    expect(getTriggerSummary(prTrigger(["labeled"]), t)).toBe("PR: labeled");
  });
});
