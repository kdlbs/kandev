import { describe, expect, it } from "vitest";
import { readinessDisplayShell, readinessProbeBody } from "./ssh-agent-readiness-card";

describe("ssh agent readiness shell selection", () => {
  it("omits shell override when the profile has no explicit shell", () => {
    expect(readinessProbeBody("")).toEqual({});
    expect(readinessProbeBody("   ")).toEqual({});
  });

  it("trims and sends explicit shell choices", () => {
    expect(readinessProbeBody(" zsh ")).toEqual({ shell: "zsh" });
  });

  it("displays the detected default when no explicit shell is saved", () => {
    expect(readinessDisplayShell("", "zsh")).toBe("zsh");
    expect(readinessDisplayShell("", "")).toBe("bash");
  });

  it("displays explicit shell choices over detected defaults", () => {
    expect(readinessDisplayShell("bash", "zsh")).toBe("bash");
  });
});
