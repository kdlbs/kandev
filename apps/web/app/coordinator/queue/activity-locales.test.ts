import { describe, expect, it } from "vitest";
import { MESSAGE_TEXT_KEY } from "./activity-text";
import en from "../../../src/locales/en/coordinator.json";
import ja from "../../../src/locales/ja/coordinator.json";
import ptPt from "../../../src/locales/pt-pt/coordinator.json";
import zhCn from "../../../src/locales/zh-cn/coordinator.json";
import zhHk from "../../../src/locales/zh-hk/coordinator.json";
import zhTw from "../../../src/locales/zh-tw/coordinator.json";

const LOCALES: Record<string, Record<string, string>> = {
  ja,
  "pt-pt": ptPt,
  "zh-cn": zhCn,
  "zh-hk": zhHk,
  "zh-tw": zhTw,
};
const catalog = en as Record<string, string>;
const KEYS = Object.keys(catalog).filter((k) => /^activity(?!Chip|Verb)/.test(k));
const placeholders = (value: string) =>
  [...value.matchAll(/{{\s*(\w+)\s*}}/g)].map((m) => m[1]).sort();

describe("What it did locale keys", () => {
  it("has the section's keys in English", () => {
    expect(KEYS.length).toBeGreaterThan(50);
    expect(KEYS).toContain("activityUndoTitle");
  });

  it("maps every undo message key to an English catalog entry", () => {
    for (const [key, ref] of Object.entries(MESSAGE_TEXT_KEY)) {
      expect(ref, key).toBe(`coordinator:${key}`);
      expect(catalog[key], key).toBeTruthy();
    }
    expect(MESSAGE_TEXT_KEY.activityConflictFeederStartsAgent).toBe(
      "coordinator:activityConflictFeederStartsAgent",
    );
  });

  it.each(Object.keys(LOCALES))("%s carries every key with the same placeholders", (locale) => {
    for (const key of KEYS) {
      const value = LOCALES[locale][key];
      expect(value, `${locale}:${key}`).toBeTruthy();
      expect(placeholders(value), `${locale}:${key}`).toEqual(placeholders(catalog[key]));
      expect(value.includes("—"), `${locale}:${key}`).toBe(false);
    }
  });
});

describe("improvement activity class label", () => {
  it("maps the improvement class to its own key, not the unknown fallback", async () => {
    const { classLabel } = await import("./activity-text");
    const t = ((key: string) => key) as never;
    expect(classLabel("improvement", t)).toBe("coordinator:activityClassImprovement");
    expect(classLabel("bogus", t)).toBe("coordinator:activityClassUnknown");
    expect(catalog.activityClassImprovement).toBeTruthy();
  });
});
