import { describe, expect, it } from "vitest";
import { sanitizeMermaidCode } from "./mermaid-utils";

describe("sanitizeMermaidCode", () => {
  it("leaves a pre-quoted bracket label with parens inside untouched", () => {
    const input = `D --> E["router.push('/github')"]`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("quotes a bracket label containing slashes alongside a pre-quoted neighbour", () => {
    const input = `A[plain] --> B[Types 'github' / 'pr' / 'dashboard']\nD --> E["router.push('/github')"]`;
    const out = sanitizeMermaidCode(input);
    expect(out).toContain(`B["Types 'github' / 'pr' / 'dashboard'"]`);
    expect(out).toContain(`E["router.push('/github')"]`);
  });

  it("leaves an init directive with single quotes untouched", () => {
    const input = `%%{init: {'theme': 'neutral'}}%%`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("quotes a bracket label containing parens", () => {
    const input = `Action[reorderSidebarViews(activeId, overId)]`;
    expect(sanitizeMermaidCode(input)).toBe(`Action["reorderSidebarViews(activeId, overId)"]`);
  });

  it("quotes a bracket label containing arrow `->`", () => {
    const input = `SSR[fetchUserSettings -> mapUserSettingsResponse]`;
    expect(sanitizeMermaidCode(input)).toBe(`SSR["fetchUserSettings -> mapUserSettingsResponse"]`);
  });

  it("quotes a standalone stadium node containing `/`", () => {
    const input = `X(/api/v1)`;
    expect(sanitizeMermaidCode(input)).toBe(`X("/api/v1")`);
  });

  it("quotes an edge label containing `/`", () => {
    const input = `A -->|/path/to/x| B`;
    expect(sanitizeMermaidCode(input)).toBe(`A -->|"/path/to/x"| B`);
  });

  it("leaves a plain stadium node alone", () => {
    const input = `Y(plain text)`;
    expect(sanitizeMermaidCode(input)).toBe(input);
  });

  it("does not corrupt parens inside a bracket label after the bracket pass quotes it", () => {
    // Pass 1 wraps `[fetch(/api/x)]` -> `["fetch(/api/x)"]`. Pass 3 must skip the
    // newly-quoted region rather than re-wrapping `(/api/x)` and producing nested quotes.
    const input = `Z[fetch(/api/x)]`;
    expect(sanitizeMermaidCode(input)).toBe(`Z["fetch(/api/x)"]`);
  });

  it("renders the full reported case 1 diagram without nested quotes", () => {
    const input = [
      `%%{init: {'theme': 'neutral'}}%%`,
      `flowchart TD`,
      `    A[User opens Cmd+K panel] --> B[Types 'github' / 'pr' / 'dashboard']`,
      `    D --> E["router.push('/github')"]`,
    ].join("\n");
    const out = sanitizeMermaidCode(input);
    expect(out).not.toContain(`("'`);
    expect(out).toContain(`E["router.push('/github')"]`);
    expect(out).toContain(`B["Types 'github' / 'pr' / 'dashboard'"]`);
  });
});
