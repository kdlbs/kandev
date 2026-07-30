import { useState } from "react";
import { act, cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { LOCATION_CHANGE_EVENT } from "@/lib/routing/navigation-event";
import { RootErrorBoundary, RouteErrorBoundary } from "./app-error-boundary";

const failure = vi.hoisted(() => ({
  location: null as "provider" | "route" | null,
}));

vi.mock("@/components/state-provider", () => ({
  StateProvider: ({ children }: { children: React.ReactNode }) => {
    if (failure.location === "provider") {
      throw new Error("private provider diagnostics");
    }
    return children;
  },
  useAppStoreApi: () => ({
    getState: () => ({ clearAuthenticated: vi.fn() }),
  }),
}));

vi.mock("@/components/plugins/plugin-modal-host", () => ({
  PluginModalHost: () => null,
}));

vi.mock("@/lib/plugins/plugin-boot-bridge", () => ({
  PluginBootBridge: () => null,
}));

vi.mock("@/lib/api/client", () => ({
  setOnUnauthorized: vi.fn(),
}));

vi.mock("./auth-gate", () => ({
  AuthGatedScreen: () => null,
  useAuthGateDecision: () => "app",
}));

vi.mock("./boot-payload", () => ({
  loadBootPayload: () => Promise.resolve({ initialState: {}, plugins: [] }),
}));

vi.mock("./app-shell", () => ({
  AppShell: ({ children }: { children: React.ReactNode }) => (
    <>
      <nav aria-label="App navigation">Navigation</nav>
      <main>{children}</main>
    </>
  ),
}));

vi.mock("./spa-routes", () => ({
  SpaRoutes: () => {
    if (failure.location === "route") {
      throw new Error("private route diagnostics");
    }
    return <div>Route content</div>;
  },
}));

async function bootApplication() {
  document.body.innerHTML = '<div id="root"></div>';
  await act(async () => {
    await import("./main");
    await Promise.resolve();
  });
}

function Boom(): never {
  throw new Error("private render diagnostics");
}

function navigate(href: string) {
  act(() => {
    window.history.pushState({}, "", href);
    window.dispatchEvent(new Event(LOCATION_CHANGE_EVENT));
  });
}

beforeEach(() => {
  vi.resetModules();
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  window.history.replaceState({}, "", "/");
});

afterEach(() => {
  failure.location = null;
  cleanup();
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe("application failure containment", () => {
  it("shows a dependency-minimal recovery alert when the state provider throws", async () => {
    failure.location = "provider";

    await bootApplication();

    const alert = await waitFor(() => screen.getByRole("alert"));
    expect(alert.textContent).toMatch(/reload/i);
    expect(alert.textContent).not.toContain("private provider diagnostics");
  });

  it("preserves shell navigation and shows recovery when a route throws", async () => {
    failure.location = "route";

    await bootApplication();

    expect(await waitFor(() => screen.getByRole("navigation"))).not.toBeNull();
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toMatch(/reload/i);
    expect(alert.textContent).not.toContain("private route diagnostics");
  });
});

describe("application recovery surfaces", () => {
  it.each([
    ["root", RootErrorBoundary],
    ["route", RouteErrorBoundary],
  ] as const)("offers a touch-safe hard reload action for a %s failure", (_, Boundary) => {
    const reload = vi.spyOn(window.location, "reload").mockImplementation(() => undefined);

    render(
      <Boundary>
        <Boom />
      </Boundary>,
    );

    const button = screen.getByRole("button", { name: "Reload" });
    expect(button.className).toContain("min-h-11");

    fireEvent.click(button);

    expect(reload).toHaveBeenCalledOnce();
  });

  it("uses full-viewport geometry for a root failure", () => {
    render(
      <RootErrorBoundary>
        <Boom />
      </RootErrorBoundary>,
    );

    expect(screen.getByRole("alert").className).toContain("min-h-dvh");
  });

  it.each([
    ["/settings?section=general", "/office?section=general"],
    ["/settings?section=general", "/settings?section=agents"],
  ])("resets an errored route after its location changes from %s to %s", (from, to) => {
    let shouldThrow = true;
    function RecoverableRoute() {
      if (shouldThrow) throw new Error("private route diagnostics");
      return <div>Recovered route</div>;
    }

    window.history.replaceState({}, "", from);
    const { rerender } = render(
      <RouteErrorBoundary>
        <RecoverableRoute />
      </RouteErrorBoundary>,
    );
    expect(screen.getByRole("alert")).not.toBeNull();

    shouldThrow = false;
    rerender(
      <RouteErrorBoundary>
        <RecoverableRoute />
      </RouteErrorBoundary>,
    );
    expect(screen.getByRole("alert")).not.toBeNull();

    navigate(to);

    expect(screen.getByText("Recovered route")).not.toBeNull();
  });

  it("does not remount healthy route state when the location changes", () => {
    function StatefulRoute() {
      const [count, setCount] = useState(0);
      return <button onClick={() => setCount((current) => current + 1)}>Count {count}</button>;
    }

    window.history.replaceState({}, "", "/settings");
    render(
      <RouteErrorBoundary>
        <StatefulRoute />
      </RouteErrorBoundary>,
    );
    fireEvent.click(screen.getByRole("button", { name: "Count 0" }));

    navigate("/office");

    expect(screen.getByRole("button", { name: "Count 1" })).not.toBeNull();
  });
});
