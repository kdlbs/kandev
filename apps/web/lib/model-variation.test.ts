import { describe, expect, it } from "vitest";
import { findUniqueModelVariation } from "./model-variation";

describe("findUniqueModelVariation", () => {
  it.each([
    {
      name: "finds one bracketed variation",
      requested: "opus",
      advertised: ["opus[1m]"],
      expected: "opus[1m]",
    },
    {
      name: "deduplicates the same variation",
      requested: "opus",
      advertised: ["opus[1m]", "opus[1m]"],
      expected: "opus[1m]",
    },
    {
      name: "keeps opaque variation text",
      requested: "opus",
      advertised: ["opus[1m, fast]"],
      expected: "opus[1m, fast]",
    },
  ])("$name", ({ requested, advertised, expected }) => {
    expect(findUniqueModelVariation(requested, advertised)).toBe(expected);
  });

  it.each([
    {
      name: "returns no candidate for an empty catalog",
      requested: "opus",
      advertised: [],
    },
    {
      name: "returns no candidate for ambiguous variations",
      requested: "opus",
      advertised: ["opus[1m]", "opus[270k]"],
    },
    {
      name: "returns no candidate for case differences",
      requested: "opus",
      advertised: ["Opus[1m]"],
    },
    {
      name: "returns no candidate for a bracketed request",
      requested: "opus[1m]",
      advertised: ["opus[2m]"],
    },
    {
      name: "returns no candidate for malformed IDs",
      requested: "opus",
      advertised: ["opus[]", "opus[1m", "opus[1m][fast]", "opus-pro[1m]"],
    },
  ])("$name", ({ requested, advertised }) => {
    expect(findUniqueModelVariation(requested, advertised)).toBeNull();
  });
});
