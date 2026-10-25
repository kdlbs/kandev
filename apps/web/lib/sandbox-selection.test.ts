import { describe, expect, it } from "vitest";
import { sandboxSelectionChoices, canSaveSandboxSelection } from "./sandbox-selection";
import type { SandboxRevision } from "./types/sandbox";

const revision = {
  id: "eligible",
  number: 2,
  revoked: false,
  policy: { name: "Work" },
} as SandboxRevision;

describe("sandbox policy selection", () => {
  it("retains a stored ineligible revision as a disabled choice", () => {
    const choices = sandboxSelectionChoices([revision], "removed", "Stored", "Inherit");
    expect(choices).toEqual([
      { value: "__inherit__", label: "Inherit" },
      { value: "eligible", label: "Work (2)" },
      { value: "removed", label: "Stored", disabled: true },
    ]);
  });

  it("refuses revoked or unavailable choices without replacing the saved value", () => {
    const revoked = { ...revision, revoked: true };
    expect(
      sandboxSelectionChoices([revoked], "eligible", "Stored", "Inherit").at(-1),
    ).toMatchObject({ value: "eligible", disabled: true });
    expect(canSaveSandboxSelection("eligible", [revoked], true)).toBe(false);
    expect(canSaveSandboxSelection("removed", [revision], true)).toBe(false);
    expect(canSaveSandboxSelection("eligible", [revision], true)).toBe(true);
  });

  it("allows explicit inheritance repair but requires an authorized ready context", () => {
    expect(canSaveSandboxSelection("", [], true)).toBe(true);
    expect(canSaveSandboxSelection("", [], false)).toBe(false);
    expect(canSaveSandboxSelection("eligible", [revision], false)).toBe(false);
  });
});
