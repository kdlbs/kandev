import { describe, expect, it } from "vitest";
import {
  filterPreviewSrcSet,
  isAllowedPreviewResourceUrl,
  sanitizePreviewAttributes,
  sanitizePreviewCss,
} from "./preview-resource-policy";

describe("preview resource policy", () => {
  it("allows embedded resources and runtime-owned blob tokens only", () => {
    const owned = new Set(["blob:preview-runtime-1"]);

    expect(isAllowedPreviewResourceUrl("data:image/svg+xml,%3Csvg%3E", owned)).toBe(true);
    expect(isAllowedPreviewResourceUrl("blob:preview-runtime-1", owned)).toBe(true);
    expect(isAllowedPreviewResourceUrl("blob:other-runtime", owned)).toBe(false);
    expect(isAllowedPreviewResourceUrl("https://example.com/image.png", owned)).toBe(false);
    expect(isAllowedPreviewResourceUrl("images/image.png", owned)).toBe(false);
  });

  it("removes navigation and executable attributes from a snapshot", () => {
    const attributes = sanitizePreviewAttributes(
      "a",
      {
        href: "https://example.com",
        onclick: "alert(1)",
        id: "link",
        title: "safe",
      },
      new Set(),
    );

    expect(attributes).toEqual({ id: "link", title: "safe" });
  });

  it("filters srcset and CSS URLs with the same deny-by-default policy", () => {
    const owned = new Set(["blob:preview-runtime-1"]);

    expect(
      filterPreviewSrcSet("data:image/png;base64,abc 1x, https://example.com/x 2x", owned),
    ).toBe("data:image/png;base64,abc 1x");
    expect(
      sanitizePreviewCss("background: url(data:image/png;base64,abc); color: red", owned),
    ).toContain("data:image/png;base64,abc");
    expect(
      sanitizePreviewCss(
        "background: url(https://example.com/x); @import url(https://example.com/y)",
        owned,
      ),
    ).not.toContain("https://");
  });
});
