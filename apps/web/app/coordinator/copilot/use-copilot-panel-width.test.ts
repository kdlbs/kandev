import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it } from "vitest";
import { PREVIEW_PANEL } from "@/lib/settings/constants";
import { COPILOT_WIDTH_STORAGE_KEY, useCopilotPanelWidth } from "./use-copilot-panel-width";

beforeEach(() => localStorage.clear());

describe("useCopilotPanelWidth", () => {
  it("defaults to the shared default width under its own storage key", () => {
    const { result } = renderHook(() => useCopilotPanelWidth());
    expect(result.current.widthPx).toBe(PREVIEW_PANEL.DEFAULT_WIDTH_PX);
    expect(COPILOT_WIDTH_STORAGE_KEY).toBe("kandev.coordinatorCopilot.width");
  });

  it("restores a stored width and persists a change without touching the board's key", () => {
    localStorage.setItem(COPILOT_WIDTH_STORAGE_KEY, JSON.stringify(640));
    const { result } = renderHook(() => useCopilotPanelWidth());
    expect(result.current.widthPx).toBe(640);
    act(() => result.current.updateWidth(720));
    expect(result.current.widthPx).toBe(720);
    expect(JSON.parse(localStorage.getItem(COPILOT_WIDTH_STORAGE_KEY) ?? "")).toBe(720);
    expect(localStorage.length).toBe(1);
  });

  it("ignores a corrupt stored value", () => {
    localStorage.setItem(COPILOT_WIDTH_STORAGE_KEY, JSON.stringify("wide"));
    const { result } = renderHook(() => useCopilotPanelWidth());
    expect(result.current.widthPx).toBe(PREVIEW_PANEL.DEFAULT_WIDTH_PX);
  });
});
