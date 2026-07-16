import { describe, expect, it } from "vitest";
import { isDraftEntryDirty, isEditorsSettingsDirty, isSetMembershipDirty } from "./settings-dirty";

describe("settings dirty comparisons", () => {
  it("compares membership without depending on array order", () => {
    expect(isSetMembershipDirty(["go", "typescript"], ["typescript", "go"])).toBe(false);
    expect(isSetMembershipDirty(["go"], ["go", "typescript"])).toBe(true);
  });

  it("marks only the changed record entry", () => {
    const draft = { go: '{"gofumpt":true}', typescript: "{}" };
    const baseline = { go: "{}", typescript: "{}" };

    expect(isDraftEntryDirty(draft, baseline, "go")).toBe(true);
    expect(isDraftEntryDirty(draft, baseline, "typescript")).toBe(false);
    expect(isDraftEntryDirty({}, {}, "python")).toBe(false);
  });

  it("compares the complete editors draft", () => {
    const baseline = {
      defaultEditorId: "vscode",
      baselineDefaultId: "vscode",
      lspAutoStartLanguages: ["go"],
      baselineLspAutoStart: ["go"],
      lspAutoInstallLanguages: ["go"],
      baselineLspAutoInstall: ["go"],
      lspConfigStrings: { go: "{}" },
      baselineLspConfigStrings: { go: "{}" },
    };

    expect(isEditorsSettingsDirty(baseline)).toBe(false);
    expect(isEditorsSettingsDirty({ ...baseline, defaultEditorId: "cursor" })).toBe(true);
  });
});
