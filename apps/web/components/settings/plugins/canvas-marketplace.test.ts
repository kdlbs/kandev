import { describe, expect, it } from "vitest";
import { resolveCanvasMarketplaceWorkspaceId } from "./canvas-marketplace";

describe("resolveCanvasMarketplaceWorkspaceId", () => {
  const firstWorkspace = "workspace-a";
  const secondWorkspace = "workspace-b";

  it("preserves an explicit valid destination when the active workspace differs", () => {
    expect(
      resolveCanvasMarketplaceWorkspaceId(secondWorkspace, firstWorkspace, [
        { id: firstWorkspace },
        { id: secondWorkspace },
      ]),
    ).toBe(secondWorkspace);
  });

  it("uses the active workspace when the current destination is unavailable", () => {
    expect(
      resolveCanvasMarketplaceWorkspaceId("workspace-missing", secondWorkspace, [
        { id: firstWorkspace },
        { id: secondWorkspace },
      ]),
    ).toBe(secondWorkspace);
  });
});
