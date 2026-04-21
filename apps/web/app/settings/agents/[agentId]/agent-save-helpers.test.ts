import { describe, it, expect } from "vitest";
import type { AgentProfile } from "@/lib/types/http";
import { isProfileDirty, type DraftProfile } from "./agent-save-helpers";

const baseProfile: AgentProfile = {
  id: "p1",
  agent_id: "a1",
  name: "Profile",
  agent_display_name: "Mock",
  model: "mock-fast",
  mode: "default",
  allow_indexing: false,
  cli_flags: [],
  cli_passthrough: false,
  created_at: "2026-01-01T00:00:00Z",
  updated_at: "2026-01-01T00:00:00Z",
};

const draftFrom = (saved: AgentProfile, overrides: Partial<DraftProfile> = {}): DraftProfile => ({
  ...saved,
  ...overrides,
});

const ALLOW_ALL_TOOLS_FLAG = "--allow-all-tools";

describe("isProfileDirty", () => {
  it("returns false when draft equals saved", () => {
    expect(isProfileDirty(draftFrom(baseProfile), baseProfile)).toBe(false);
  });

  it("returns true when only mode changes", () => {
    const draft = draftFrom(baseProfile, { mode: "plan-mock" });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("treats undefined mode as equal to empty string", () => {
    const saved: AgentProfile = { ...baseProfile, mode: undefined };
    const draft = draftFrom(saved, { mode: "" });
    expect(isProfileDirty(draft, saved)).toBe(false);
  });

  it("returns true when mode changes from empty to a value", () => {
    const saved: AgentProfile = { ...baseProfile, mode: "" };
    const draft = draftFrom(saved, { mode: "plan-mock" });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns true when mode changes from a value to empty (cleared)", () => {
    const saved: AgentProfile = { ...baseProfile, mode: "plan-mock" };
    const draft = draftFrom(saved, { mode: "" });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns true when there is no saved profile", () => {
    expect(isProfileDirty(draftFrom(baseProfile))).toBe(true);
  });

  it("returns true when cli_flags list changes", () => {
    const draft = draftFrom(baseProfile, {
      cli_flags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("returns true when a cli_flag enabled state changes", () => {
    const saved: AgentProfile = {
      ...baseProfile,
      cli_flags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: false, description: "" }],
    };
    const draft = draftFrom(saved, {
      cli_flags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns false when cli_flags are equal", () => {
    const flags = [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "desc" }];
    const saved: AgentProfile = { ...baseProfile, cli_flags: flags };
    const draft = draftFrom(saved, { cli_flags: [...flags] });
    expect(isProfileDirty(draft, saved)).toBe(false);
  });
});
