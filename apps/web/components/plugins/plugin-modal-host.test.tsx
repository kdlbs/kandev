import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { pluginModalManager } from "@/lib/plugins/modal-manager";
import { PluginModalHost } from "./plugin-modal-host";

function cleanupModals(pluginId: string) {
  pluginModalManager.closeAllForPlugin(pluginId);
}

describe("PluginModalHost", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  it("renders nothing when no plugin has an open modal", () => {
    const { container } = render(<PluginModalHost />);
    expect(container.innerHTML).toBe("");
  });

  it("renders an open modal's title and content", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "My Modal",
      content: () => <div data-testid="modal-content">Hello</div>,
    });

    render(<PluginModalHost />);

    expect(screen.getByText("My Modal")).not.toBeNull();
    expect(screen.getByTestId("modal-content")).not.toBeNull();
  });

  it("renders a modal description inside the host-owned header", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Link Bitbucket pull request",
      description: "Use a Bitbucket pull request URL for this task.",
      content: () => <div data-testid="modal-content">Hello</div>,
    } as never);

    render(<PluginModalHost />);

    expect(screen.getByText("Use a Bitbucket pull request URL for this task.")).not.toBeNull();
  });

  it("keeps dialog content in a local scroll body while retaining the close control", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Growing modal",
      content: () => <div data-testid="modal-content">Long plugin content</div>,
    });

    render(<PluginModalHost />);

    const dialog = screen.getByTestId(/^plugin-modal-dialog-/);
    const body = screen.getByTestId(/^plugin-modal-body-/);
    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(dialog.contains(body)).toBe(true);
    expect(screen.getByRole("button", { name: "Close" })).not.toBeNull();
  });

  it("keeps titleless task-link content in the bounded scroll row", () => {
    pluginModalManager.openTaskLinkDialog("plugin-a", {
      content: () => <div data-testid="modal-content">Long task-link content</div>,
    });

    render(<PluginModalHost />);

    const dialog = screen.getByTestId(/^plugin-modal-dialog-/);
    const body = screen.getByTestId(/^plugin-modal-body-/);
    expect(dialog.getAttribute("data-layout")).toBe("contained");
    expect(body.className).toContain("row-start-2");
    expect(screen.getByRole("dialog", { name: "Plugin dialog" })).toBeTruthy();
  });

  it("does not add a close control to a nondismissible dialog", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Locked modal",
      content: () => <div>Content</div>,
      dismissible: false,
    });

    render(<PluginModalHost />);

    expect(screen.getByTestId(/^plugin-modal-dialog-/)).not.toBeNull();
    expect(screen.queryByRole("button", { name: "Close" })).toBeNull();
  });

  it("renders a host-owned drawer when the plugin requests mobile presentation", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Link pull request",
      content: () => <div data-testid="drawer-content">Mobile action</div>,
      presentation: "drawer",
    });

    render(<PluginModalHost />);

    expect(document.querySelector('[data-slot="drawer-content"]')).not.toBeNull();
    expect(screen.getByTestId("drawer-content")).not.toBeNull();
  });

  it("gives title-less plugin surfaces an accessible fallback name", () => {
    pluginModalManager.openModal("plugin-a", {
      content: () => <div>Untitled content</div>,
      presentation: "drawer",
    });

    render(<PluginModalHost />);

    expect(screen.getByRole("dialog", { name: "Plugin dialog" })).not.toBeNull();
  });

  it("removes the modal from the DOM once its handle is closed", () => {
    const handle = pluginModalManager.openModal("plugin-a", {
      content: () => <div data-testid="modal-content">Hello</div>,
    });

    render(<PluginModalHost />);
    expect(screen.getByTestId("modal-content")).not.toBeNull();

    act(() => {
      handle.close();
    });

    expect(screen.queryByTestId("modal-content")).toBeNull();
  });
});

/**
 * The host mounts as a sibling of `<AppShell/>` (src/main.tsx), so it is
 * outside the app-wide `TooltipProvider` in app/layout.tsx. Without a
 * provider of its own, a `Tooltip` anywhere in plugin modal content throws on
 * render and the whole modal is lost to the error boundary.
 *
 * jsdom does not reliably open a Radix tooltip from synthetic hover (see
 * apps/web/CLAUDE.md), so the open assertion goes through focus. Pointer
 * hover is a Playwright concern.
 */
describe("PluginModalHost — tooltips in modal content", () => {
  afterEach(() => {
    cleanup();
    cleanupModals("plugin-a");
  });

  function TooltipContentComponent() {
    return (
      <Tooltip>
        <TooltipTrigger data-testid="tooltip-trigger">Trigger</TooltipTrigger>
        <TooltipContent>Helpful hint</TooltipContent>
      </Tooltip>
    );
  }

  it("renders modal content containing a Tooltip without throwing", () => {
    pluginModalManager.openModal("plugin-a", {
      title: "Tooltip Modal",
      content: TooltipContentComponent,
    });

    render(<PluginModalHost />);

    expect(screen.getByTestId("tooltip-trigger")).not.toBeNull();
  });

  it("opens the tooltip on focus, proving a TooltipProvider is in scope", async () => {
    pluginModalManager.openModal("plugin-a", { content: TooltipContentComponent });

    render(<PluginModalHost />);
    fireEvent.focus(screen.getByTestId("tooltip-trigger"));

    await waitFor(() => {
      expect(screen.getAllByText("Helpful hint").length).toBeGreaterThan(0);
    });
  });
});
