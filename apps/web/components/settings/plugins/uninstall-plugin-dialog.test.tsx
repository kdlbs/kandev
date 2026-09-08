import { useRef, type ComponentProps } from "react";
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { PluginUninstallConfirmation } from "./uninstall-plugin-dialog";
import type { PluginRecord } from "@/lib/types/plugins";

afterEach(cleanup);

function plugin(): PluginRecord {
  return {
    id: "acme-tools",
    api_version: 1,
    version: "1.0.0",
    display_name: "Acme Tools",
    description: "",
    author: "acme",
    categories: [],
    capabilities: {},
    status: "active",
    install_path: "/plugins/acme-tools",
    signed: true,
    installed_at: "2026-01-01T00:00:00Z",
    restart_count: 0,
  };
}

function Confirmation({ isFinePointer }: { isFinePointer: boolean }) {
  const anchorRef = useRef<HTMLButtonElement>(null);
  const props = {
    target: plugin(),
    open: true,
    isFinePointer,
    anchorRef,
    onOpenChange: vi.fn(),
    onCancel: vi.fn(),
    onConfirm: vi.fn(),
  } satisfies ComponentProps<typeof PluginUninstallConfirmation>;

  return (
    <>
      <button ref={anchorRef} type="button" data-testid="plugin-uninstall-trigger">
        Uninstall
      </button>
      <PluginUninstallConfirmation {...props} />
    </>
  );
}

describe("plugin uninstall confirmation", () => {
  it("uses an anchored non-modal confirmation on fine pointers", () => {
    render(<Confirmation isFinePointer />);

    expect(screen.getByTestId("plugin-uninstall-confirm-popover")).toBeTruthy();
    expect(document.querySelector('[data-slot="dialog-overlay"]')).toBeNull();
    expect(screen.getByRole("dialog").textContent).toContain("Acme Tools");
    expect(screen.getByTestId("plugin-uninstall-confirm").getAttribute("aria-label")).toBe(
      "Confirm uninstall",
    );
  });

  it("keeps phone confirmation inline with touch-sized actions", () => {
    render(<Confirmation isFinePointer={false} />);

    expect(screen.getByTestId("plugin-uninstall-inline-confirmation")).toBeTruthy();
    expect(document.querySelector('[data-slot="dialog-overlay"]')).toBeNull();
    expect(screen.getByTestId("plugin-uninstall-confirm").className).toContain("h-11");
    expect(screen.getByTestId("plugin-uninstall-confirm").className).toContain("min-w-11");
  });
});
