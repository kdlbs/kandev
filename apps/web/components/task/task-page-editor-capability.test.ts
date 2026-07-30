import { describe, expect, it } from "vitest";
import { getEmbeddedVscodeSupported } from "./task-page-editor-capability";

describe("getEmbeddedVscodeSupported", () => {
  it("fails closed while session capability data is missing", () => {
    expect(getEmbeddedVscodeSupported(null)).toBe(false);
    expect(getEmbeddedVscodeSupported({})).toBe(false);
  });

  it("uses the active session capability only when it is explicitly true", () => {
    expect(getEmbeddedVscodeSupported({ capabilities: { embedded_vscode: true } })).toBe(true);
    expect(getEmbeddedVscodeSupported({ capabilities: { embedded_vscode: false } })).toBe(false);
  });
});
