import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { PluginRepoLink, isHttpUrl } from "./plugin-repo-link";

afterEach(() => cleanup());

describe("isHttpUrl", () => {
  it("accepts http and https, rejects other schemes and blanks", () => {
    expect(isHttpUrl("https://github.com/kdlbs/kandev-plugin-acme")).toBe(true);
    expect(isHttpUrl("http://example.com")).toBe(true);
    expect(isHttpUrl("  https://example.com  ")).toBe(true);
    expect(isHttpUrl("javascript:alert(1)")).toBe(false);
    expect(isHttpUrl("ftp://example.com")).toBe(false);
    expect(isHttpUrl("")).toBe(false);
  });
});

describe("PluginRepoLink", () => {
  it("renders an external link for an http(s) URL", () => {
    render(<PluginRepoLink url="https://github.com/kdlbs/kandev-plugin-acme" />);
    const link = screen.getByTestId("plugin-repo-link");
    expect(link.getAttribute("href")).toBe("https://github.com/kdlbs/kandev-plugin-acme");
    expect(link.getAttribute("target")).toBe("_blank");
    expect(link.getAttribute("rel")).toBe("noreferrer");
  });

  it("renders nothing for a javascript: (or other non-http) scheme", () => {
    render(<PluginRepoLink url="javascript:alert(document.cookie)" />);
    expect(screen.queryByTestId("plugin-repo-link")).toBeNull();
  });

  it("renders nothing for an empty URL", () => {
    render(<PluginRepoLink url="" />);
    expect(screen.queryByTestId("plugin-repo-link")).toBeNull();
  });
});
