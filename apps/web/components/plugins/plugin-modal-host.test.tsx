import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
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
