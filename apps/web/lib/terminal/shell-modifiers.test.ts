import { describe, it, expect, beforeEach } from "vitest";
import { useShellModifiersStore, isActive } from "./shell-modifiers";

beforeEach(() => {
  useShellModifiersStore.getState().reset();
});

describe("shell-modifiers store", () => {
  it("toggles ctrl through off → latched → sticky → off", () => {
    const { toggleCtrl } = useShellModifiersStore.getState();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });

    toggleCtrl();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: false });

    toggleCtrl();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: true });

    toggleCtrl();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
  });

  it("consumeCtrl clears a latched (non-sticky) modifier", () => {
    useShellModifiersStore.getState().toggleCtrl();
    useShellModifiersStore.getState().consumeCtrl();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: false, sticky: false });
  });

  it("consumeCtrl is a no-op when sticky", () => {
    useShellModifiersStore.getState().toggleCtrl();
    useShellModifiersStore.getState().toggleCtrl();
    useShellModifiersStore.getState().consumeCtrl();
    expect(useShellModifiersStore.getState().ctrl).toEqual({ latched: true, sticky: true });
  });

  it("shift mirrors ctrl behavior", () => {
    const { toggleShift, consumeShift } = useShellModifiersStore.getState();
    toggleShift();
    expect(useShellModifiersStore.getState().shift.latched).toBe(true);
    consumeShift();
    expect(useShellModifiersStore.getState().shift.latched).toBe(false);
  });

  it("isActive returns true for latched or sticky", () => {
    expect(isActive({ latched: false, sticky: false })).toBe(false);
    expect(isActive({ latched: true, sticky: false })).toBe(true);
    expect(isActive({ latched: false, sticky: true })).toBe(true);
    expect(isActive({ latched: true, sticky: true })).toBe(true);
  });
});
