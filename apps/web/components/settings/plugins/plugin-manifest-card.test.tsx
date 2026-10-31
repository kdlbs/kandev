import { afterEach, describe, expect, it } from "vitest";
import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { PluginManifestCard } from "./plugin-manifest-card";
import type { PluginRecord } from "@/lib/types/plugins";

afterEach(() => cleanup());

function plugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme-plugin",
    api_version: 2,
    version: "1.4.2",
    display_name: "Acme Plugin",
    description: "An acme plugin",
    author: "kandev",
    categories: ["tools"],
    capabilities: {},
    status: "active",
    install_path: "/tmp/plugins/acme-plugin/1.4.2",
    signed: false,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

/** The manifest author row, scoped so another hyphenated or author-named value cannot satisfy it. */
function authorRow(): HTMLElement {
  const label = within(screen.getByTestId("plugin-manifest-card")).getByText("Declared author");
  const row = label.parentElement;
  if (!row) throw new Error("manifest author row has no container");
  return row;
}

describe("PluginManifestCard author declaration", () => {
  it("labels the manifest author as declared and keeps the original value", () => {
    render(<PluginManifestCard plugin={plugin({ author: "kandev" })} />);

    expect(authorRow().textContent).toContain("Declared author");
    expect(authorRow().textContent).toContain("kandev");
    expect(screen.queryByText("Verified publisher")).toBeNull();
  });

  it("preserves the manifest author in the raw manifest output", () => {
    render(<PluginManifestCard plugin={plugin({ author: "kandev" })} />);

    fireEvent.click(screen.getByRole("button", { name: "View raw" }));

    expect(screen.getByTestId("plugin-manifest-raw").textContent).toContain('"author": "kandev"');
  });

  it("keeps the unavailable marker when the manifest declares no author", () => {
    render(<PluginManifestCard plugin={plugin({ author: "" })} />);

    expect(authorRow().textContent).toContain("Declared author");
    expect(authorRow().textContent).toContain("-");
  });
});
