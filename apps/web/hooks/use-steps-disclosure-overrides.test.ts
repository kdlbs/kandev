import { act, renderHook } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { useStepsDisclosureOverrides } from "./use-steps-disclosure-overrides";

describe("useStepsDisclosureOverrides — the override-reset rule", () => {
  it("starts with an empty override map", () => {
    const { result } = renderHook(() => useStepsDisclosureOverrides(true, "mobile"));
    expect(result.current.overrides).toEqual({});
  });

  it("records an explicit toggle", () => {
    const { result } = renderHook(() => useStepsDisclosureOverrides(true, "mobile"));
    act(() => result.current.toggleDisclosure("wf-a", false));
    expect(result.current.overrides).toEqual({ "wf-a": true });
  });

  it("resets the map after the surface closes (trigger 1)", () => {
    const { result, rerender } = renderHook(
      ({ open }: { open: boolean }) => useStepsDisclosureOverrides(open, "mobile"),
      { initialProps: { open: true } },
    );
    act(() => result.current.toggleDisclosure("wf-a", false));
    expect(result.current.overrides).toEqual({ "wf-a": true });

    rerender({ open: false });

    expect(result.current.overrides).toEqual({});
  });

  it("does NOT reset while the surface stays open and the surface key is unchanged", () => {
    const { result, rerender } = renderHook(
      ({ surfaceKey }: { surfaceKey: string }) => useStepsDisclosureOverrides(true, surfaceKey),
      { initialProps: { surfaceKey: "mobile" } },
    );
    act(() => result.current.toggleDisclosure("wf-a", false));

    rerender({ surfaceKey: "mobile" });

    expect(result.current.overrides).toEqual({ "wf-a": true });
  });

  it("resets after a breakpoint crossing while the surface is held open (trigger 2: mobile -> tablet -> mobile)", () => {
    const { result, rerender } = renderHook(
      ({ surfaceKey }: { surfaceKey: string }) => useStepsDisclosureOverrides(true, surfaceKey),
      { initialProps: { surfaceKey: "mobile" } },
    );
    act(() => result.current.toggleDisclosure("wf-a", false));
    expect(result.current.overrides).toEqual({ "wf-a": true });

    // Widen into the tablet range with the drawer's `open` prop true throughout.
    rerender({ surfaceKey: "tablet" });
    // Narrow back below 768px, still held open.
    rerender({ surfaceKey: "mobile" });

    expect(result.current.overrides).toEqual({});
  });

  it("does not reset on close->close (no transition) or on the very first render", () => {
    const { result } = renderHook(() => useStepsDisclosureOverrides(false, "mobile"));
    expect(result.current.overrides).toEqual({});
  });
});
