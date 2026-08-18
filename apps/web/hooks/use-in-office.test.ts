import { afterEach, describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

let officeEnabled = false;
let scope = { mode: "unknown" as "office" | "kanban" | "unknown" };

vi.mock("@/hooks/domains/features/use-feature", () => ({
  useFeature: () => officeEnabled,
}));
vi.mock("@/components/workspace-scope-provider", () => ({
  useWorkspaceScope: () => scope,
}));

import { useInOffice, useOfficeModeState } from "./use-in-office";

function mode(): string {
  return renderHook(() => useOfficeModeState()).result.current;
}

function inOffice(): boolean {
  return renderHook(() => useInOffice()).result.current;
}

describe("useOfficeModeState", () => {
  afterEach(() => {
    officeEnabled = false;
    scope = { mode: "unknown" };
  });

  it("follows the workspace, not the route", () => {
    // The whole point of the change: mode is a property of the selected
    // workspace, so no pathname is consulted here at all.
    officeEnabled = true;
    scope = { mode: "office" };
    expect(mode()).toBe("office");

    scope = { mode: "kanban" };
    expect(mode()).toBe("kanban");
  });

  it("reports unknown until the workspace list has hydrated", () => {
    // Distinct from "kanban": callers painting mode-specific chrome hold on
    // this rather than rendering one mode and swapping to the other.
    officeEnabled = true;
    scope = { mode: "unknown" };
    expect(mode()).toBe("unknown");
  });

  it("degrades an office workspace to kanban when the feature is disabled", () => {
    // Such a build registers no /api/v1/office/* routes, so Office chrome would
    // link to surfaces that cannot load.
    officeEnabled = false;
    scope = { mode: "office" };
    expect(mode()).toBe("kanban");
  });

  it("still reports unknown when the feature is disabled and nothing has resolved", () => {
    officeEnabled = false;
    scope = { mode: "unknown" };
    expect(mode()).toBe("unknown");
  });
});

describe("useInOffice", () => {
  afterEach(() => {
    officeEnabled = false;
    scope = { mode: "unknown" };
  });

  it("is true only for a resolved office workspace", () => {
    officeEnabled = true;
    scope = { mode: "office" };
    expect(inOffice()).toBe(true);
  });

  it("is false for kanban, for unknown, and for a disabled feature", () => {
    officeEnabled = true;
    scope = { mode: "kanban" };
    expect(inOffice()).toBe(false);

    scope = { mode: "unknown" };
    expect(inOffice()).toBe(false);

    officeEnabled = false;
    scope = { mode: "office" };
    expect(inOffice()).toBe(false);
  });
});
