import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { MarketplaceEntryRow } from "./marketplace-entry-row";
import type { MarketplaceEntry } from "@/lib/types/plugins";

afterEach(() => cleanup());

function entry(overrides: Partial<MarketplaceEntry> = {}): MarketplaceEntry {
  return {
    id: "acme",
    name: "Acme",
    description: "desc",
    author: "acme",
    categories: [],
    icon_url: "",
    repo_url: "https://github.com/kdlbs/kandev-plugin-acme",
    version: "1.0.0",
    min_kandev_version: "",
    package_url: "https://ex/acme-1.0.0.tar.gz",
    package_sha256: "",
    stars: 3,
    updated_at: "",
    install_state: "available",
    source_id: "official",
    source_name: "Kandev Official",
    ...overrides,
  };
}

const noop = () => undefined;

describe("MarketplaceEntryRow repo link", () => {
  it("renders an http(s) repo link", () => {
    render(<MarketplaceEntryRow entry={entry()} busy={false} onInstall={noop} />);
    const link = screen.getByText("Repo").closest("a");
    expect(link?.getAttribute("href")).toBe("https://github.com/kdlbs/kandev-plugin-acme");
  });

  it("does NOT render a repo link for a javascript: (or other non-http) scheme", () => {
    // A malicious/compromised source could set repo_url to a javascript: URL.
    render(
      <MarketplaceEntryRow
        entry={entry({ repo_url: "javascript:alert(document.cookie)" })}
        busy={false}
        onInstall={noop}
      />,
    );
    expect(screen.queryByText("Repo")).toBeNull();
  });

  it("renders unknown (null) stars as a dash, not zero", () => {
    render(<MarketplaceEntryRow entry={entry({ stars: null })} busy={false} onInstall={noop} />);
    expect(screen.getByText("—")).toBeTruthy();
  });
});
