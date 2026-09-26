import { describe, expect, it } from "vitest";
import { applyTitlePrefix, composeTitle } from "./document-title";

function fakeDocument(title: string): Document {
  return { title } as Document;
}

describe("composeTitle", () => {
  it.each([
    ["TEST", "TEST Kandev"],
    ["  staging  ", "staging Kandev"],
    ["", "Kandev"],
    ["   ", "Kandev"],
    [undefined, "Kandev"],
  ])("composes %o as %s", (prefix, expected) => {
    expect(composeTitle(prefix)).toBe(expected);
  });
});

describe("applyTitlePrefix", () => {
  it("prefixes the document title", () => {
    const doc = fakeDocument("Kandev");
    applyTitlePrefix("TEST", doc);
    expect(doc.title).toBe("TEST Kandev");
  });

  it("trims surrounding whitespace from the prefix", () => {
    const doc = fakeDocument("Kandev");
    applyTitlePrefix("  TEST  ", doc);
    expect(doc.title).toBe("TEST Kandev");
  });

  it.each([undefined, "", "   "])("leaves the title alone for %o", (prefix) => {
    const doc = fakeDocument("Kandev");
    applyTitlePrefix(prefix, doc);
    expect(doc.title).toBe("Kandev");
  });
});
