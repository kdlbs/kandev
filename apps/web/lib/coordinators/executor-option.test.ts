import { describe, expect, it } from "vitest";
import { withUnavailableExecutorOption } from "./executor-option";

describe("withUnavailableExecutorOption", () => {
  const options = [{ value: "profile-1", label: "worktree" }];

  it("returns the options unchanged when the value matches one of them", () => {
    expect(withUnavailableExecutorOption(options, "profile-1", "Removed")).toBe(options);
  });

  it("returns the options unchanged when there is no value yet", () => {
    expect(withUnavailableExecutorOption(options, "", "Removed")).toBe(options);
  });

  it("prepends a disabled synthetic option when the stored value matches none (B11)", () => {
    const result = withUnavailableExecutorOption(options, "profile-missing", "Removed");
    expect(result).toEqual([
      { value: "profile-missing", label: "Removed", disabled: true, disabledReason: "Removed" },
      ...options,
    ]);
  });
});
