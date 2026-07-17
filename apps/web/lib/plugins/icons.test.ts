import { describe, expect, it } from "vitest";
import { IconPuzzle, IconTicket } from "@tabler/icons-react";
import { lookupPluginIcon, resolvePluginIcon } from "./icons";

describe("plugin icons", () => {
  it("looks up a known icon name", () => {
    expect(lookupPluginIcon("ticket")).toBe(IconTicket);
    expect(resolvePluginIcon("ticket")).toBe(IconTicket);
  });

  it("returns undefined from lookupPluginIcon for unknown or missing names", () => {
    expect(lookupPluginIcon("not-an-icon")).toBeUndefined();
    expect(lookupPluginIcon(undefined)).toBeUndefined();
  });

  it("falls back to the puzzle glyph from resolvePluginIcon", () => {
    expect(resolvePluginIcon("not-an-icon")).toBe(IconPuzzle);
    expect(resolvePluginIcon(undefined)).toBe(IconPuzzle);
  });
});
