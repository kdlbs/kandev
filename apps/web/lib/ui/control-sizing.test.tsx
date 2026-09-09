import { describe, expect, it } from "vitest";

import { buttonVariants } from "@kandev/ui/button";
import { controlSizingClassName } from "@kandev/ui/control-sizing";

describe("shared control sizing", () => {
  it("keeps the ordinary button at desktop density and enlarges touch contexts", () => {
    const classes = buttonVariants({ size: "default" });

    expect(classes).toContain("h-7");
    expect(classes).toContain("max-md:h-11");
    expect(classes).toContain("[@media(pointer:coarse)]:h-11");
  });

  it("keeps the compact button variant at 24px on fine-pointer desktop", () => {
    const classes = buttonVariants({ size: "sm" });

    expect(classes).toContain("h-6");
    expect(classes).not.toContain("max-md:h-11");
    expect(classes).not.toContain("[@media(pointer:coarse)]:h-11");
  });

  it("shares the responsive standard classes with non-button controls", () => {
    expect(controlSizingClassName("standard")).toBe(
      "h-7 max-md:h-11 [@media(pointer:coarse)]:h-11",
    );
  });
});
