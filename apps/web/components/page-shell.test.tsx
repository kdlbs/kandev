import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageShell } from "./page-shell";

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  AppStatusDrawerTrigger: () => null,
}));

vi.mock("@/components/navigation/app-nav-sheet", () => ({
  AppNavSheet: ({ pageNav }: { pageNav?: React.ReactNode }) => (
    <div data-testid="nav-sheet-mock">{pageNav}</div>
  ),
}));

const affordance = { mode: "phone" as "none" | "phone" | "always", href: "/?home=overview" };

vi.mock("@/hooks/use-home-affordance", () => ({
  useHomeAffordance: (variant: string) =>
    variant === "root" ? { ...affordance, mode: "none" } : affordance,
}));

describe("PageShell", () => {
  afterEach(() => {
    cleanup();
    affordance.mode = "phone";
    affordance.href = "/?home=overview";
  });

  it("renders no main landmark — AppShell owns it", () => {
    const { container } = render(<PageShell title="Stats">content</PageShell>);

    expect(container.querySelector("main")).toBeNull();
    expect(container.querySelector("header")).not.toBeNull();
  });

  it("injects the nav sheet and hands it the pageNav", () => {
    render(
      <PageShell title="Settings" pageNav={<span data-testid="tree" />}>
        content
      </PageShell>,
    );

    expect(screen.getByTestId("nav-sheet-mock")).not.toBeNull();
    expect(screen.getByTestId("tree")).not.toBeNull();
  });

  it("keeps the nav sheet ahead of page-supplied leading content", () => {
    render(
      <PageShell title="Office" leading={<span data-testid="slot" />}>
        content
      </PageShell>,
    );

    const header = screen.getByRole("banner");
    const order = [...header.querySelectorAll("[data-testid]")].map((el) =>
      el.getAttribute("data-testid"),
    );
    expect(order.indexOf("nav-sheet-mock")).toBeLessThan(order.indexOf("slot"));
  });

  it("applies the resolved home affordance to the topbar crumb", () => {
    affordance.mode = "always";
    affordance.href = "/office";
    render(<PageShell title="Costs">content</PageShell>);

    const home = screen.getByTestId("topbar-phone-home");
    expect(home.className).not.toContain("md:hidden");
    expect(home.querySelector("a")?.getAttribute("href")).toBe("/office");
  });

  it("wraps children in a scroll container with the given testid and classes", () => {
    render(
      <PageShell title="Settings" contentTestId="settings-scroll-container" contentClassName="p-4">
        <span>content</span>
      </PageShell>,
    );

    const scroller = screen.getByTestId("settings-scroll-container");
    expect(scroller.className).toContain("overflow-y-auto");
    expect(scroller.className).toContain("p-4");
    expect(scroller.textContent).toBe("content");
  });

  it("renders children bare with scroll=none", () => {
    const { container } = render(
      <PageShell title="Board" scroll="none" contentTestId="never-rendered">
        <span data-testid="full-bleed" />
      </PageShell>,
    );

    expect(screen.getByTestId("full-bleed")).not.toBeNull();
    expect(container.querySelector('[data-testid="never-rendered"]')).toBeNull();
  });

  it("forwards the topbar testid", () => {
    render(
      <PageShell title="Office" topbarTestId="office-topbar">
        content
      </PageShell>,
    );

    expect(screen.getByTestId("office-topbar").tagName).toBe("HEADER");
  });
});
