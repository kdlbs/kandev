import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { PluginErrorDiagnostic } from "./plugin-error-diagnostic";
import type { PluginRecord } from "@/lib/types/plugins";

afterEach(() => cleanup());

function plugin(overrides: Partial<PluginRecord> = {}): PluginRecord {
  return {
    id: "acme",
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme",
    description: "",
    author: "acme",
    categories: [],
    capabilities: {},
    status: "error",
    install_path: "/p",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
    ...overrides,
  };
}

describe("PluginErrorDiagnostic", () => {
  it("renders the normalized message and machine-readable failure time", () => {
    render(
      <PluginErrorDiagnostic
        plugin={plugin({
          last_error: "plugins/runtime: handshake failed",
          last_error_at: "2026-08-02T12:34:56Z",
        })}
      />,
    );

    expect(screen.getByRole("alert").textContent).toContain("plugins/runtime: handshake failed");
    expect(screen.getByRole("time").getAttribute("dateTime")).toBe("2026-08-02T12:34:56Z");
  });

  it("renders nothing when there is no persisted diagnostic", () => {
    const { container } = render(<PluginErrorDiagnostic plugin={plugin()} />);
    expect(container.innerHTML).toBe("");
  });
});
