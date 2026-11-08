import { describe, expect, it } from "vitest";
import { isHttpUrl } from "./answer-sources";

describe("isHttpUrl", () => {
  it("accepts http and https", () => {
    expect(isHttpUrl("https://example.com/a")).toBe(true);
    expect(isHttpUrl("http://example.com")).toBe(true);
    expect(isHttpUrl("HTTPS://EXAMPLE.COM")).toBe(true);
  });

  it("rejects every other scheme and non-URLs", () => {
    expect(isHttpUrl("javascript:alert(1)")).toBe(false);
    expect(isHttpUrl("JaVaScRiPt:alert(1)")).toBe(false);
    expect(isHttpUrl("data:text/html,x")).toBe(false);
    expect(isHttpUrl("ftp://example.com")).toBe(false);
    expect(isHttpUrl("//example.com")).toBe(false);
    expect(isHttpUrl("docs/adr/0007.md:42")).toBe(false);
    expect(isHttpUrl("")).toBe(false);
  });
});
