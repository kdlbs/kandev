import { describe, expect, it } from "vitest";

import {
  curatedFlagEnabled,
  isCuratedFlagMatch,
  setCuratedFlagEnabled,
} from "./cli-flags-field-model";

const skipPermissionsFlag = "--dangerously-skip-permissions";
const modeWithValue = "--mode 'safe value'";

describe("curated CLI flag choices", () => {
  it("treats any enabled exact match as enabled", () => {
    const flags = [
      { flag: skipPermissionsFlag, description: "legacy off", enabled: false },
      { flag: skipPermissionsFlag, description: "legacy on", enabled: true },
    ];

    expect(curatedFlagEnabled(flags, skipPermissionsFlag, false)).toBe(true);
  });

  it("matches the existing exact curated flag text", () => {
    expect(isCuratedFlagMatch(skipPermissionsFlag, skipPermissionsFlag)).toBe(true);
    expect(isCuratedFlagMatch("--profile", skipPermissionsFlag)).toBe(false);
  });

  it("matches a curated flag with its separate value", () => {
    const flags = [
      { flag: modeWithValue, description: "custom mode", enabled: true },
      { flag: "--verbose", description: "verbose", enabled: true },
    ];

    expect(isCuratedFlagMatch(flags[0].flag, modeWithValue)).toBe(true);
    expect(setCuratedFlagEnabled(flags, modeWithValue, "mode", false)).toEqual([
      { flag: "--verbose", description: "verbose", enabled: true },
      { flag: modeWithValue, description: "custom mode", enabled: false },
    ]);
  });

  it("matches whitespace-padded curated rows consistently", () => {
    const paddedFlag = ` ${skipPermissionsFlag} `;
    const flags = [
      { flag: paddedFlag, description: "legacy bypass", enabled: true },
      { flag: "--profile", description: "profile", enabled: true },
    ];

    expect(isCuratedFlagMatch(paddedFlag, skipPermissionsFlag)).toBe(true);
    expect(curatedFlagEnabled(flags, skipPermissionsFlag, false)).toBe(true);
    expect(setCuratedFlagEnabled(flags, skipPermissionsFlag, "permission bypass", false)).toEqual([
      { flag: "--profile", description: "profile", enabled: true },
      { flag: skipPermissionsFlag, description: "legacy bypass", enabled: false },
    ]);
  });

  it("matches quoted and bundled curated flag tokens and preserves unrelated arguments", () => {
    const flags = [
      {
        flag: '"' + skipPermissionsFlag + '"',
        description: "quoted permission mode",
        enabled: true,
      },
      {
        flag: "--verbose " + skipPermissionsFlag + " --profile 'two words'",
        description: "bundled permission mode",
        enabled: true,
      },
      { flag: "--profile", description: "profile", enabled: true },
    ];

    expect(curatedFlagEnabled(flags, skipPermissionsFlag, false)).toBe(true);
    expect(setCuratedFlagEnabled(flags, skipPermissionsFlag, "permission bypass", false)).toEqual([
      {
        flag: "--verbose --profile 'two words'",
        description: "bundled permission mode",
        enabled: true,
      },
      { flag: "--profile", description: "profile", enabled: true },
      { flag: skipPermissionsFlag, description: "bundled permission mode", enabled: false },
    ]);
  });

  it("collapses every exact match when saving one effective choice", () => {
    const flags = [
      { flag: skipPermissionsFlag, description: "legacy off", enabled: false },
      { flag: "--profile", description: "profile", enabled: true },
      { flag: skipPermissionsFlag, description: "legacy on", enabled: true },
    ];

    expect(setCuratedFlagEnabled(flags, skipPermissionsFlag, "permission bypass", false)).toEqual([
      { flag: "--profile", description: "profile", enabled: true },
      { flag: skipPermissionsFlag, description: "legacy on", enabled: false },
    ]);
  });
});
