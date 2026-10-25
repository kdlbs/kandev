import { describe, expect, it } from "vitest";
import { conditionSettings, findTriggerInfo } from "./plugin-condition";
import type { AutomationTrigger, TriggerTypeInfo } from "@/lib/types/automation";
describe("plugin condition identity", () => {
  it("distinguishes conditions with the same internal trigger type", () => {
    const infos = ["push", "merge"].map((key) => ({
      type: "plugin_event",
      plugin: { plugin_id: "bitbucket", condition: { key } },
    })) as TriggerTypeInfo[];
    const trigger = {
      id: "trigger",
      automation_id: "automation",
      enabled: true,
      created_at: "",
      updated_at: "",
      last_evaluated_at: null,
      type: "plugin_event",
      config: { plugin_id: "bitbucket", condition_key: "merge" },
    } as AutomationTrigger;
    expect(findTriggerInfo(trigger, infos)).toBe(infos[1]);
    expect(
      findTriggerInfo({ ...trigger, config: { ...trigger.config, plugin_id: "other" } }, infos),
    ).toBeUndefined();
  });
  it("keeps settings scoped and rejects array-shaped settings", () => {
    expect(conditionSettings({ settings: ["bad"] })).toEqual({});
    expect(conditionSettings({ plugin_id: "p", settings: { repository: "repo" } })).toEqual({
      repository: "repo",
    });
  });
});
