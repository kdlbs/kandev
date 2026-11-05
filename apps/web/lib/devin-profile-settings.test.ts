import { describe, expect, it } from "vitest";

import {
  DEVIN_AUTONOMOUS_MODE,
  DEVIN_SANDBOX_ENV_KEY,
  DEVIN_SMART_MODE,
  devinPermissionConflict,
  devinSandboxPatch,
  devinSandboxPreference,
  hasCustomDevinSandboxValue,
  newDevinProfileDefaults,
} from "@/lib/devin-profile-settings";

describe("newDevinProfileDefaults", () => {
  it("defaults a new Devin draft to Smart with an explicit disabled sandbox", () => {
    expect(newDevinProfileDefaults({})).toEqual({
      mode: DEVIN_SMART_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "false" }],
    });
  });

  it("keeps explicit modes and existing overrides", () => {
    expect(newDevinProfileDefaults({ mode: DEVIN_AUTONOMOUS_MODE })).toEqual({
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "false" }],
    });
    expect(
      newDevinProfileDefaults({
        envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
      }),
    ).toEqual({
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
    });
  });
});

describe("sandbox preference", () => {
  it("distinguishes inherited, explicit, and custom values", () => {
    expect(devinSandboxPreference([])).toBe("inherit");
    expect(devinSandboxPreference(undefined)).toBe("inherit");
    expect(devinSandboxPreference([{ key: DEVIN_SANDBOX_ENV_KEY, value: "false" }])).toBe(
      "disabled",
    );
    expect(devinSandboxPreference([{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }])).toBe("enabled");
    expect(devinSandboxPreference([{ key: DEVIN_SANDBOX_ENV_KEY, value: "inherit" }])).toBe(
      "inherit",
    );
    expect(devinSandboxPreference([{ key: DEVIN_SANDBOX_ENV_KEY, secret_id: "secret-1" }])).toBe(
      "inherit",
    );
    expect(hasCustomDevinSandboxValue([{ key: DEVIN_SANDBOX_ENV_KEY, value: "inherit" }])).toBe(
      true,
    );
    expect(hasCustomDevinSandboxValue([{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }])).toBe(false);
  });
});

describe("devinSandboxPatch", () => {
  it("enabling requests Autonomous and preserves unrelated entries", () => {
    expect(
      devinSandboxPatch(
        {
          mode: DEVIN_SMART_MODE,
          envVars: [
            { key: "OTHER", value: "kept" },
            { key: DEVIN_SANDBOX_ENV_KEY, value: "false" },
          ],
        },
        "enabled",
      ),
    ).toEqual({
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [
        { key: "OTHER", value: "kept" },
        { key: DEVIN_SANDBOX_ENV_KEY, value: "true" },
      ],
    });
  });

  it("disabling Autonomous falls back to Smart only when it is advertised", () => {
    const profile = {
      mode: DEVIN_AUTONOMOUS_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }],
    };
    expect(devinSandboxPatch(profile, "disabled").mode).toBe(DEVIN_SMART_MODE);
    expect(devinSandboxPatch(profile, "disabled", { smartAvailable: false }).mode).toBe("");
  });

  it("inheriting removes the override and keeps a non-Autonomous mode", () => {
    expect(
      devinSandboxPatch(
        { mode: "bypass", envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }] },
        "inherit",
      ),
    ).toEqual({ mode: "bypass", envVars: [] });
  });

  it("never rewrites a custom value until the user replaces it", () => {
    const profile = {
      mode: DEVIN_SMART_MODE,
      envVars: [{ key: DEVIN_SANDBOX_ENV_KEY, value: "inherit" }],
    };
    expect(devinSandboxPreference(profile.envVars)).toBe("inherit");
    expect(devinSandboxPatch(profile, "disabled").envVars).toEqual([
      { key: DEVIN_SANDBOX_ENV_KEY, value: "false" },
    ]);
  });
});

describe("devinPermissionConflict", () => {
  it("mirrors the backend rejection of explicitly contradicting pairs", () => {
    expect(
      devinPermissionConflict(DEVIN_SMART_MODE, [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }]),
    ).toBe(true);
    expect(devinPermissionConflict("bypass", [{ key: DEVIN_SANDBOX_ENV_KEY, value: "true" }])).toBe(
      true,
    );
    expect(
      devinPermissionConflict(DEVIN_AUTONOMOUS_MODE, [
        { key: DEVIN_SANDBOX_ENV_KEY, value: "false" },
      ]),
    ).toBe(true);
    expect(
      devinPermissionConflict(DEVIN_AUTONOMOUS_MODE, [
        { key: DEVIN_SANDBOX_ENV_KEY, value: "true" },
      ]),
    ).toBe(false);
    expect(devinPermissionConflict(DEVIN_SMART_MODE, [])).toBe(false);
  });
});
