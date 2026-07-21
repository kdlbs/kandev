import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { pluginRegistry } from "@/lib/plugins/registry";
import type { PluginRouteRegistration } from "@/lib/plugins/registry";
import { PluginPageFrame, resolvePluginPageChrome } from "./plugin-page";

const PLUGIN_ID = "plugin-a";
const PLUGIN_PATH = "/plugins/hello";
const PLUGIN_NAME = "Hello Plugin";

function Page() {
  return null;
}

function registration(options?: PluginRouteRegistration["options"]): PluginRouteRegistration {
  return { pluginId: PLUGIN_ID, path: PLUGIN_PATH, Component: Page, options };
}

describe("resolvePluginPageChrome", () => {
  it("returns null when the registration opted out via topbar: false", () => {
    expect(resolvePluginPageChrome(registration({ topbar: false }), [], "Hello")).toBeNull();
  });

  it("derives the title from the nav item registered for the same path", () => {
    const chrome = resolvePluginPageChrome(
      registration(),
      [{ id: "nav", label: "Hello Nav", path: PLUGIN_PATH, icon: "ticket" }],
      PLUGIN_NAME,
    );
    expect(chrome).toMatchObject({ title: "Hello Nav", icon: "ticket" });
  });

  it("falls back to the plugin display name, then the route path", () => {
    expect(resolvePluginPageChrome(registration(), [], PLUGIN_NAME)?.title).toBe(PLUGIN_NAME);
    expect(resolvePluginPageChrome(registration(), [], undefined)?.title).toBe(PLUGIN_PATH);
  });

  it("prefers explicit chrome config over every derived default", () => {
    const chrome = resolvePluginPageChrome(
      registration({
        topbar: { title: "Custom", subtitle: "Sub", icon: "chart", backHref: "/x", backLabel: "X" },
      }),
      [{ id: "nav", label: "Nav Label", path: PLUGIN_PATH, icon: "ticket" }],
      PLUGIN_NAME,
    );
    expect(chrome).toEqual({
      title: "Custom",
      subtitle: "Sub",
      icon: "chart",
      backHref: "/x",
      backLabel: "X",
      Actions: undefined,
    });
  });
});

describe("PluginPageFrame", () => {
  afterEach(() => {
    cleanup();
    pluginRegistry.unregisterPlugin(PLUGIN_ID);
  });

  it("renders a kandev topbar with the derived title by default", () => {
    pluginRegistry.forPlugin(PLUGIN_ID, PLUGIN_NAME);

    render(
      <PluginPageFrame registration={registration()}>
        <div data-testid="plugin-content" />
      </PluginPageFrame>,
    );

    expect(screen.getByRole("banner")).not.toBeNull();
    expect(screen.getByText(PLUGIN_NAME)).not.toBeNull();
    expect(screen.getByTestId("plugin-content")).not.toBeNull();
  });

  it("renders the children bare when the route opted out of the topbar", () => {
    render(
      <PluginPageFrame registration={registration({ topbar: false })}>
        <div data-testid="plugin-content" />
      </PluginPageFrame>,
    );

    expect(screen.queryByRole("banner")).toBeNull();
    expect(screen.getByTestId("plugin-content")).not.toBeNull();
  });

  it("renders configured subtitle and a plugin-provided actions component", () => {
    function Actions() {
      return <button data-testid="plugin-topbar-action">Sync</button>;
    }

    render(
      <PluginPageFrame
        registration={registration({
          topbar: { title: "Custom", subtitle: "Tickets", actions: Actions },
        })}
      >
        <div />
      </PluginPageFrame>,
    );

    expect(screen.getByText("Custom")).not.toBeNull();
    expect(screen.getByText("Tickets")).not.toBeNull();
    expect(screen.getByTestId("plugin-topbar-action")).not.toBeNull();
  });
});
