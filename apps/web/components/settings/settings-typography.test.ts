import { describe, expect, it } from "vitest";

import { SETTINGS_TYPOGRAPHY } from "./settings-typography";
import { settingsActionClassName, settingsControlClassName } from "./settings-control";

describe("settings typography contract", () => {
  it("defines one role scale for page, section, card, and field copy", () => {
    expect(SETTINGS_TYPOGRAPHY.pageTitle).toContain("text-2xl");
    expect(SETTINGS_TYPOGRAPHY.sectionTitle).toContain("text-lg");
    expect(SETTINGS_TYPOGRAPHY.cardTitle).toContain("text-base");
    expect(SETTINGS_TYPOGRAPHY.fieldLabel).toContain("text-xs");
    expect(SETTINGS_TYPOGRAPHY.fieldDescription).toContain("text-xs/relaxed");
    expect(SETTINGS_TYPOGRAPHY.error).toContain("text-sm/relaxed");
  });

  it("uses the md breakpoint for compact desktop settings controls", () => {
    expect(SETTINGS_TYPOGRAPHY.control).toBe("text-sm md:text-xs");
    expect(SETTINGS_TYPOGRAPHY.mobileAction).toBe("min-h-11 text-sm md:min-h-7 md:text-xs");
  });

  it("gives editable controls and actions a mobile hitbox", () => {
    expect(settingsControlClassName()).toContain("min-h-11");
    expect(settingsControlClassName()).toContain("md:text-xs");
    expect(settingsActionClassName()).toContain("md:min-h-7");
  });
});
