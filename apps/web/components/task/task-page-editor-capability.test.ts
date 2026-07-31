import { describe, expect, it, vi } from "vitest";
import { renderHook } from "@testing-library/react";

// Hoisted so the vi.mock factory below, which is registered above module
// initialization, never closes over an uninitialized binding.
const { setEmbeddedVscodeSupport } = vi.hoisted(() => ({
  setEmbeddedVscodeSupport: vi.fn(),
}));

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: unknown) => unknown) => selector({ setEmbeddedVscodeSupport }),
}));

import {
  getEmbeddedVscodeSupported,
  useEmbeddedVscodeSupport,
} from "./task-page-editor-capability";

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

const SUPPORTED = { capabilities: { embedded_vscode: true } };
const UNSUPPORTED = { capabilities: { embedded_vscode: false } };

describe("useEmbeddedVscodeSupport", () => {
  it("returns and publishes the capability for the active session", () => {
    setEmbeddedVscodeSupport.mockClear();
    const { result } = renderHook(() => useEmbeddedVscodeSupport("session-1", SUPPORTED));

    expect(result.current).toBe(true);
    expect(setEmbeddedVscodeSupport).toHaveBeenCalledWith("session-1", true);
  });

  it("republishes when the session's capability changes", () => {
    setEmbeddedVscodeSupport.mockClear();
    const { rerender } = renderHook(({ status }) => useEmbeddedVscodeSupport("session-1", status), {
      initialProps: { status: SUPPORTED },
    });
    rerender({ status: UNSUPPORTED });

    expect(setEmbeddedVscodeSupport).toHaveBeenLastCalledWith("session-1", false);
  });

  it("fails closed and publishes nothing without an active session", () => {
    setEmbeddedVscodeSupport.mockClear();
    const { result } = renderHook(() => useEmbeddedVscodeSupport(null, SUPPORTED));

    expect(result.current).toBe(true);
    expect(setEmbeddedVscodeSupport).not.toHaveBeenCalled();
  });
});
