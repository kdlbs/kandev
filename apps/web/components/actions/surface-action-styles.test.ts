import { describe, expect, it } from "vitest";
import { surfaceActionClassName, surfaceActionGroupClassName } from "./surface-action-styles";

describe("surface action styles", () => {
  it("keeps status action groups bounded to one compact bar row", () => {
    const groupClassName = surfaceActionGroupClassName("status-bar", "desktop");

    expect(groupClassName).toContain("max-w-72");
    expect(groupClassName).toContain("flex-nowrap");
    expect(groupClassName).toContain("[&>[data-slot=surface-action]]:shrink");
  });

  it("keeps the compact status control at 24px without a duplicate override", () => {
    const actionClassName = surfaceActionClassName("status-bar", "desktop", true);

    expect(actionClassName).toBe("h-6 max-w-72 gap-1 px-1 text-[11px]");
  });
});
