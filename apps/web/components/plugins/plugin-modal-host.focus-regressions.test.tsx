import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { pluginModalManager } from "@/lib/plugins/modal-manager";
import { PluginModalHost } from "./plugin-modal-host";

describe("PluginModalHost review regressions", () => {
  afterEach(() => {
    cleanup();
    pluginModalManager.closeAllForPlugin("plugin-a");
    pluginModalManager.closeAllForPlugin("plugin-b");
    if (vi.isFakeTimers()) {
      act(() => vi.runOnlyPendingTimers());
      vi.useRealTimers();
    }
  });

  it("returns focus to an opener inside an aria-disabled group", async () => {
    let closeHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(
      <>
        <PluginModalHost />
        <div aria-disabled="true">
          <button data-testid="modal-opener">Open modal</button>
        </div>
      </>,
    );

    const opener = screen.getByTestId("modal-opener");
    opener.focus();
    act(() => {
      closeHandle = pluginModalManager.openModal("plugin-a", {
        content: () => <button data-testid="modal-action">Modal action</button>,
      });
    });
    await waitFor(() => expect(document.activeElement).toBe(screen.getByTestId("modal-action")));
    act(() => closeHandle?.close());

    await waitFor(() => expect(document.activeElement).toBe(opener));
  });

  it("preserves a foreground dropdown menu when a background modal closes", async () => {
    let backgroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    let foregroundHandle: ReturnType<typeof pluginModalManager.openModal> | undefined;
    render(
      <>
        <PluginModalHost />
        <button data-testid="page-opener">Open plugin modal</button>
      </>,
    );

    screen.getByTestId("page-opener").focus();
    act(() => {
      backgroundHandle = pluginModalManager.openModal("plugin-a", {
        title: "Background modal",
        content: () => <button>Background action</button>,
      });
      foregroundHandle = pluginModalManager.openModal("plugin-b", {
        title: "Foreground modal",
        content: ForegroundDropdownMenuContent,
      });
    });

    fireEvent.pointerDown(screen.getByTestId("foreground-menu-trigger"), {
      button: 0,
      ctrlKey: false,
      pointerType: "mouse",
    });
    const menuItem = await screen.findByTestId("foreground-menu-item");
    menuItem.focus();
    expect(document.activeElement).toBe(menuItem);

    vi.useFakeTimers();
    act(() => backgroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());

    const menu = screen.getByTestId("foreground-menu-content");
    expect(menu.getAttribute("data-state")).toBe("open");
    expect(document.activeElement).toBe(menuItem);
    act(() => foregroundHandle?.close());
    act(() => vi.runOnlyPendingTimers());
    vi.useRealTimers();
  });

  it("returns focus to the opener when owner cleanup removes the focused modal", async () => {
    render(
      <>
        <PluginModalHost />
        <button data-testid="owned-modal-opener">Open modal</button>
      </>,
    );
    const opener = screen.getByTestId("owned-modal-opener");
    opener.focus();
    act(() => {
      pluginModalManager.openModal("plugin-a", {
        title: "Owned modal",
        content: () => <button data-testid="owned-action">Owned action</button>,
      });
    });
    const ownedAction = await screen.findByTestId("owned-action");
    await waitFor(() => expect(document.activeElement).toBe(ownedAction));

    vi.useFakeTimers();
    act(() => pluginModalManager.closeAllForPlugin("plugin-a"));
    act(() => vi.runOnlyPendingTimers());

    expect(screen.queryByRole("dialog", { name: "Owned modal" })).toBeNull();
    expect(document.activeElement).toBe(opener);
    vi.useRealTimers();
  });
});

function ForegroundDropdownMenuContent() {
  const [open, setOpen] = useState(false);
  return (
    <DropdownMenu modal={false} open={open} onOpenChange={setOpen}>
      <DropdownMenuTrigger asChild>
        <button data-testid="foreground-menu-trigger">Open menu</button>
      </DropdownMenuTrigger>
      <DropdownMenuContent data-testid="foreground-menu-content">
        <DropdownMenuItem data-testid="foreground-menu-item">Menu action</DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
