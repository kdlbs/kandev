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
