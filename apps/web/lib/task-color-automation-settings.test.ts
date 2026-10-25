import { describe, expect, it } from "vitest";
import {
  DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION,
  parseSidebarTaskColorAutomation,
  type SidebarTaskColorAutomation,
} from "./task-color-automation-settings";

const fixedRule = {
  id: "blocked",
  enabled: true,
  condition: { dimension: "task_state", value: "BLOCKED", label: "Blocked" },
  output: { kind: "fixed", color: "red" },
} as const;

describe("parseSidebarTaskColorAutomation", () => {
  it("uses a disabled empty default when the setting is missing or malformed", () => {
    expect(parseSidebarTaskColorAutomation(undefined)).toEqual(
      DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION,
    );
    expect(parseSidebarTaskColorAutomation({ enabled: true, rules: "not-an-array" })).toEqual(
      DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION,
    );
    expect(
      parseSidebarTaskColorAutomation({
        enabled: true,
        rules: [{ ...fixedRule, condition: { ...fixedRule.condition, value: null } }],
      }),
    ).toEqual(DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION);
  });

  it("preserves disabled incomplete rules and complete replacement order", () => {
    const value: SidebarTaskColorAutomation = {
      enabled: true,
      rules: [
        fixedRule,
        {
          id: "unfinished",
          enabled: false,
          condition: { dimension: "repository", value: null, label: "Missing repository" },
          output: { kind: "fixed", color: "cyan" },
        },
      ],
    };
    expect(parseSidebarTaskColorAutomation(value)).toEqual(value);
  });

  it("rejects duplicate ids, unsupported dimensions, and unsupported colors", () => {
    for (const invalid of [
      { enabled: true, rules: [{ ...fixedRule }, { ...fixedRule }] },
      {
        enabled: true,
        rules: [{ ...fixedRule, condition: { dimension: "future", value: "x", label: "x" } }],
      },
      { enabled: true, rules: [{ ...fixedRule, output: { kind: "fixed", color: "teal" } }] },
    ]) {
      expect(parseSidebarTaskColorAutomation(invalid)).toEqual(
        DEFAULT_SIDEBAR_TASK_COLOR_AUTOMATION,
      );
    }
  });
});
