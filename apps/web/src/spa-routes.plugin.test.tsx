import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { pluginRegistry } from "@/lib/plugins/registry";
import { resolveSpaRoute, SpaRoutes } from "./spa-routes";

const PLUGIN_ID = "plugin-a";
const PLUGIN_PATH = "/plugins/hello";

function cleanupPlugins(...pluginIds: string[]) {
  pluginIds.forEach((id) => pluginRegistry.unregisterPlugin(id));
}

describe("resolveSpaRoute — plugin fallthrough", () => {
  afterEach(() => cleanupPlugins(PLUGIN_ID));

  it("falls back to the kanban route when no plugin owns the path", () => {
    expect(resolveSpaRoute("/plugins/hello", new URLSearchParams())).toEqual({
      kind: "kanban",
      workspaceId: undefined,
      workflowId: undefined,
      taskId: undefined,
      sessionId: undefined,
    });
  });

  it("resolves to the plugin route once a plugin registers the path", () => {
    function HelloPage() {
      return null;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerRoute(PLUGIN_PATH, HelloPage);

    expect(resolveSpaRoute(PLUGIN_PATH, new URLSearchParams())).toEqual({
      kind: "plugin",
      path: PLUGIN_PATH,
    });
  });

  it("does not shadow a first-class static route with the same-looking plugin path", () => {
    function TasksOverride() {
      return null;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerRoute("/tasks", TasksOverride);

    expect(resolveSpaRoute("/tasks", new URLSearchParams())).toEqual({ kind: "tasks" });
  });
});

describe("SpaRoutes — plugin route rendering", () => {
  afterEach(() => {
    cleanup();
    cleanupPlugins(PLUGIN_ID);
    window.history.pushState({}, "", "/");
  });

  it("renders the registered plugin component inside the app shell for a matching path", () => {
    function HelloPage() {
      return <div data-testid="plugin-hello-page">Hello from plugin</div>;
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerRoute(PLUGIN_PATH, HelloPage);
    window.history.pushState({}, "", PLUGIN_PATH);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    expect(screen.getByTestId("plugin-hello-page")).not.toBeNull();
  });

  it("wraps a plugin route in the kandev title bar by default", () => {
    function HelloPage() {
      return <div data-testid="plugin-hello-page" />;
    }
    pluginRegistry.forPlugin(PLUGIN_ID, "Hello Plugin").registerRoute(PLUGIN_PATH, HelloPage);
    window.history.pushState({}, "", PLUGIN_PATH);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    expect(screen.getByRole("banner")).not.toBeNull();
    expect(screen.getByText("Hello Plugin")).not.toBeNull();
    expect(screen.getByTestId("plugin-hello-page")).not.toBeNull();
  });

  it("renders the route full-bleed when it opts out via topbar: false", () => {
    function HelloPage() {
      return <div data-testid="plugin-hello-page" />;
    }
    pluginRegistry
      .forPlugin(PLUGIN_ID, "Hello Plugin")
      .registerRoute(PLUGIN_PATH, HelloPage, { topbar: false });
    window.history.pushState({}, "", PLUGIN_PATH);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    expect(screen.queryByRole("banner")).toBeNull();
    expect(screen.getByTestId("plugin-hello-page")).not.toBeNull();
  });

  it("renders a fallback instead of white-screening when the plugin route component throws", () => {
    const errorSpy = vi.spyOn(console, "error").mockImplementation(() => undefined);
    function ThrowingPage(): never {
      throw new Error("boom");
    }
    pluginRegistry.forPlugin(PLUGIN_ID).registerRoute(PLUGIN_PATH, ThrowingPage);
    window.history.pushState({}, "", PLUGIN_PATH);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    expect(screen.getByText(/this plugin page failed to load/i)).not.toBeNull();
    errorSpy.mockRestore();
  });
});
