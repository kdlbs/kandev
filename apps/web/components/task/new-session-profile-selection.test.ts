import { describe, expect, it } from "vitest";
import { resolveNewSessionProfileSelection } from "./new-session-profile-selection";

const profiles = [{ id: "profile-a" }, { id: "profile-b" }, { id: "profile-c" }];

describe("resolveNewSessionProfileSelection", () => {
  it("selects a compatible task-session recent profile over the current session", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: profiles,
      recentProfileIds: ["profile-b", "profile-c"],
      currentProfileId: "profile-a",
    });

    expect(selection).toEqual({
      profileId: "profile-b",
      source: "recent",
      profileExplicit: true,
    });
  });

  it("prioritizes a compatible handoff target over recent use", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: profiles,
      recentProfileIds: ["profile-b"],
      currentProfileId: "profile-a",
      handoffProfileId: "profile-c",
    });

    expect(selection).toEqual({
      profileId: "profile-c",
      source: "handoff",
      profileExplicit: true,
    });
  });

  it("skips ineligible recent profiles and keeps the current profile fallback implicit", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: [profiles[0], profiles[2]],
      recentProfileIds: ["profile-b"],
      currentProfileId: "profile-a",
    });

    expect(selection).toEqual({
      profileId: "profile-a",
      source: "current",
      profileExplicit: false,
    });
  });

  it("keeps the current profile fallback when recent history is unavailable", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: profiles,
      currentProfileId: "profile-a",
    });

    expect(selection).toEqual({
      profileId: "profile-a",
      source: "current",
      profileExplicit: false,
    });
  });

  it("keeps the current profile fallback when recent history is empty", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: profiles,
      recentProfileIds: [],
      currentProfileId: "profile-a",
    });

    expect(selection).toEqual({
      profileId: "profile-a",
      source: "current",
      profileExplicit: false,
    });
  });

  it("uses source order when history and the current profile are unavailable", () => {
    const selection = resolveNewSessionProfileSelection({
      compatibleProfiles: [profiles[1], profiles[2]],
      recentProfileIds: ["missing"],
      currentProfileId: "profile-a",
    });

    expect(selection).toEqual({
      profileId: "profile-b",
      source: "source",
      profileExplicit: false,
    });
  });
});
