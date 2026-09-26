import { describe, expect, it } from "vitest";
import { PREVIEW_PANEL } from "./constants";
import { getRenderedPreviewPanelWidth } from "./preview-panel-width";

describe("getRenderedPreviewPanelWidth", () => {
  it("passes a fine-pointer chosen width through when at or above the fine minimum", () => {
    expect(getRenderedPreviewPanelWidth(500, true)).toBe(500);
    expect(getRenderedPreviewPanelWidth(PREVIEW_PANEL.MIN_WIDTH_PX, true)).toBe(
      PREVIEW_PANEL.MIN_WIDTH_PX,
    );
  });

  it("floors a fine-pointer chosen width below the fine minimum", () => {
    expect(getRenderedPreviewPanelWidth(1, true)).toBe(PREVIEW_PANEL.MIN_WIDTH_PX);
  });

  it("floors a coarse-pointer chosen width below the coarse minimum", () => {
    expect(getRenderedPreviewPanelWidth(PREVIEW_PANEL.MIN_WIDTH_PX, false)).toBe(
      PREVIEW_PANEL.COARSE_MIN_WIDTH_PX,
    );
    expect(getRenderedPreviewPanelWidth(1, false)).toBe(PREVIEW_PANEL.COARSE_MIN_WIDTH_PX);
  });

  it("passes a coarse-pointer chosen width through when at or above the coarse minimum", () => {
    expect(getRenderedPreviewPanelWidth(500, false)).toBe(500);
    expect(getRenderedPreviewPanelWidth(PREVIEW_PANEL.COARSE_MIN_WIDTH_PX, false)).toBe(
      PREVIEW_PANEL.COARSE_MIN_WIDTH_PX,
    );
  });
});
