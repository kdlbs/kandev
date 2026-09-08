import { describe, expect, it } from "vitest";
import { resolveSpaRoute } from "./spa-routes";

describe("resolveSpaRoute canvas routes", () => {
  it("resolves an encoded direct canvas id before plugin fallthrough", () => {
    expect(
      resolveSpaRoute("/canvases/canvas%2Fone", new URLSearchParams(), {
        canvasesEnabled: true,
      }),
    ).toEqual({
      kind: "canvas",
      canvasId: "canvas/one",
    });
  });

  it("resolves workspace canvas settings separately from generic settings", () => {
    expect(
      resolveSpaRoute("/settings/workspaces/ws%2Fone/canvases", new URLSearchParams(), {
        canvasesEnabled: true,
      }),
    ).toEqual({
      kind: "canvasSettings",
      workspaceId: "ws/one",
    });
  });

  it("does not resolve canvas routes while the feature is disabled", () => {
    expect(resolveSpaRoute("/canvases/canvas-1", new URLSearchParams()).kind).not.toBe("canvas");
    expect(
      resolveSpaRoute("/settings/workspaces/ws-1/canvases", new URLSearchParams()).kind,
    ).not.toBe("canvasSettings");
  });

  it("does not treat incomplete canvas paths as canvas routes", () => {
    expect(resolveSpaRoute("/canvases", new URLSearchParams()).kind).toBe("kanban");
  });
});
