import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { SpaRoutes } from "./spa-routes";

vi.mock("./settings-routes", () => new Promise(() => undefined));
vi.mock("./office-routes", () => new Promise(() => undefined));

afterEach(() => {
  cleanup();
  window.history.replaceState({}, "", "/");
});

describe("lazy SPA route loading", () => {
  it.each([
    ["/settings", "settings"],
    ["/office", "office"],
  ])("shows one accessible status while %s is loading", (pathname, routeName) => {
    window.history.replaceState({}, "", pathname);

    render(
      <StateProvider>
        <SpaRoutes />
      </StateProvider>,
    );

    const statuses = screen.getAllByRole("status");
    expect(statuses).toHaveLength(1);
    expect(statuses[0].textContent?.toLowerCase()).toContain(`loading ${routeName}`);
  });
});
