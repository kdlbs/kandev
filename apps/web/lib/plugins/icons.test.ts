import { describe, expect, it } from "vitest";
import { IconPuzzle, IconTicket } from "@tabler/icons-react";
import { lookupPluginIcon, resolvePluginIcon } from "./icons";

describe("plugin icons", () => {
  it("looks up a known icon name", () => {
    expect(lookupPluginIcon("ticket")).toBe(IconTicket);
    expect(resolvePluginIcon("ticket")).toBe(IconTicket);
  });

  it("does not require host-owned provider brand icons", () => {
    expect(lookupPluginIcon("bitbucket")).toBeUndefined();
    expect(resolvePluginIcon("bitbucket")).toBe(IconPuzzle);
  });

  it("passes through a plugin-owned icon component", () => {
    const PluginIcon = () => null;

    expect(lookupPluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
    expect(resolvePluginIcon(PluginIcon as unknown as string)).toBe(PluginIcon);
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
