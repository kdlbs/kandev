import { describe, it, expect } from "vitest";
import { agentProfileId as toAgentProfileId, type AgentProfile } from "@/lib/types/http";
import { isProfileDirty, type DraftProfile } from "./agent-save-helpers";

const baseProfile: AgentProfile = {
  id: toAgentProfileId("p1"),
  agentId: "a1",
  name: "Profile",
  agentDisplayName: "Mock",
  model: "mock-fast",
  mode: "default",
  allowIndexing: false,
  cliFlags: [],
  cliPassthrough: false,
  createdAt: "2026-01-01T00:00:00Z",
  updatedAt: "2026-01-01T00:00:00Z",
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

  it("returns true when cliFlags list changes", () => {
    const draft = draftFrom(baseProfile, {
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, baseProfile)).toBe(true);
  });

  it("returns true when a cliFlag enabled state changes", () => {
    const saved: AgentProfile = {
      ...baseProfile,
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: false, description: "" }],
    };
    const draft = draftFrom(saved, {
      cliFlags: [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "" }],
    });
    expect(isProfileDirty(draft, saved)).toBe(true);
  });

  it("returns false when cliFlags are equal", () => {
    const flags = [{ flag: ALLOW_ALL_TOOLS_FLAG, enabled: true, description: "desc" }];
    const saved: AgentProfile = { ...baseProfile, cliFlags: flags };
    const draft = draftFrom(saved, { cliFlags: [...flags] });
    expect(isProfileDirty(draft, saved)).toBe(false);
  });
});
