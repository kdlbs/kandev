import { describe, expect, it } from "vitest";
import { resolveSelectionCommentDockPosition } from "./selection-comment-dock-position";

const viewport = { left: 0, top: 0, right: 400, bottom: 800, width: 400, height: 800 };

describe("resolveSelectionCommentDockPosition", () => {
  it("keeps the dock inside its owner and above the composer and keyboard", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 40, top: 100, right: 360, bottom: 760, width: 320, height: 660 },
      boundaryRect: { left: 40, top: 620, right: 360, bottom: 760, width: 320, height: 140 },
      viewportRect: { left: 0, top: 0, right: 400, bottom: 650, width: 400, height: 650 },
      dockWidth: 160,
      dockHeight: 56,
      safeArea: { top: 0, right: 0, bottom: 12, left: 0 },
    });

    expect(position).toEqual({
      left: 120,
      top: 564,
      width: 160,
      height: 56,
      clearance: 56,
    });
  });

  it("clips horizontal placement to a panned visual viewport", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 0, top: 100, right: 500, bottom: 700, width: 500, height: 600 },
      viewportRect: { left: 60, top: 20, right: 390, bottom: 750, width: 330, height: 730 },
      dockWidth: 360,
      dockHeight: 56,
      safeArea: { top: 0, right: 8, bottom: 0, left: 8 },
    });

    expect(position).toEqual({
      left: 68,
      top: 644,
      width: 314,
      height: 56,
      clearance: 56,
    });
  });

  it("converts viewport coordinates into a scrolled portal container", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 120, top: 80, right: 420, bottom: 600, width: 300, height: 520 },
      viewportRect: viewport,
      dockWidth: 140,
      dockHeight: 56,
      safeArea: { top: 0, right: 0, bottom: 0, left: 0 },
      portalOrigin: { left: 100, top: 50, scrollLeft: 10, scrollTop: 20 },
    });

    expect(position).toEqual({
      left: 100,
      top: 514,
      width: 140,
      height: 56,
      clearance: 56,
    });
  });

  it("keeps clipped viewport clearance independent from portal coordinates", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 120, top: 80, right: 420, bottom: 700, width: 300, height: 620 },
      viewportRect: { left: 0, top: 0, right: 400, bottom: 500, width: 400, height: 500 },
      dockWidth: 140,
      dockHeight: 56,
      portalOrigin: { left: 100, top: 50, scrollLeft: 10, scrollTop: 20 },
    });

    expect(position).toMatchObject({ left: 100, top: 414, clearance: 256 });
  });
});

describe("resolveSelectionCommentDockPosition clearance", () => {
  it("applies safe-area clearance only when the owner reaches the viewport edge", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 0, top: 100, right: 400, bottom: 500, width: 400, height: 400 },
      viewportRect: viewport,
      dockWidth: 140,
      dockHeight: 56,
      safeArea: { top: 0, right: 0, bottom: 24, left: 0 },
    });

    expect(position?.top).toBe(444);
  });

  it("reserves the owner's layout overlap above a clipped visual viewport", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 0, top: 80, right: 400, bottom: 700, width: 400, height: 620 },
      viewportRect: { left: 0, top: 0, right: 400, bottom: 500, width: 400, height: 500 },
      dockWidth: 160,
      dockHeight: 56,
    });

    expect(position).toMatchObject({ top: 444, clearance: 256 });
  });

  it("measures clearance to a composer boundary before the keyboard-clipped viewport", () => {
    const position = resolveSelectionCommentDockPosition({
      ownerRect: { left: 0, top: 80, right: 400, bottom: 700, width: 400, height: 620 },
      boundaryRect: { left: 0, top: 620, right: 400, bottom: 700, width: 400, height: 80 },
      viewportRect: { left: 0, top: 0, right: 400, bottom: 500, width: 400, height: 500 },
      dockWidth: 160,
      dockHeight: 56,
    });

    expect(position).toMatchObject({ top: 444, clearance: 176 });
  });

  it("hides the dock when the owner is detached or has insufficient visible room", () => {
    const detached = resolveSelectionCommentDockPosition({
      ownerRect: null,
      viewportRect: viewport,
      dockWidth: 140,
      dockHeight: 56,
    });
    const tooShort = resolveSelectionCommentDockPosition({
      ownerRect: { left: 0, top: 790, right: 400, bottom: 800, width: 400, height: 10 },
      viewportRect: viewport,
      dockWidth: 140,
      dockHeight: 56,
    });

    expect(detached).toBeNull();
    expect(tooShort).toBeNull();
  });
});
