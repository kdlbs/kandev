import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { RowTitleLink } from "./row-title-link";

const longTitle =
  "§7 item 4 (km-product#27) — EntityLocker is not the REQ-011 FR-8 answer lock (no change proposed; recording the distinction)";

afterEach(cleanup);

describe("RowTitleLink", () => {
  it("keeps a long title inside its column instead of overlapping row actions", () => {
    render(<RowTitleLink href="https://gitlab.example.com/x/-/issues/3" title={longTitle} />);

    const link = screen.getByRole("link");
    // Block-level flex: an inline-flex link sizes to content and overflows the row.
    expect(link.className).toContain("flex");
    expect(link.className).not.toContain("inline-flex");
    expect(link.className).toContain("min-w-0");

    const text = screen.getByText(longTitle);
    expect(text.className).toContain("truncate");
  });

  it("exposes the full title on hover since the text may be clipped", () => {
    render(<RowTitleLink href="https://gitlab.example.com/x/-/issues/3" title={longTitle} />);

    expect(screen.getByRole("link").getAttribute("title")).toBe(longTitle);
  });
});
