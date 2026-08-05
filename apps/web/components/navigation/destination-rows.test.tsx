import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { DestinationRows } from "./destination-rows";
import type { ResolvedDestination } from "@/lib/navigation/types";

vi.mock("@/lib/routing/client-router", () => ({
  usePathname: () => "/",
}));

function Glyph({ className }: { className?: string }) {
  return <svg className={className} data-testid="glyph" />;
}

const STATS: ResolvedDestination = {
  id: "stats",
  label: "Stats",
  icon: Glyph,
  section: "insights",
  href: "/stats",
};

const PLUGIN_PAGE: ResolvedDestination = {
  id: "hello",
  label: "Hello",
  icon: Glyph,
  section: "plugins",
  href: "/plugins/hello",
  source: "plugin",
};

function renderRows(destinations: ResolvedDestination[], pluginTestIdPrefix?: string) {
  const onNavigate = vi.fn();
  const view = render(
    <DestinationRows
      destinations={destinations}
      onNavigate={onNavigate}
      {...(pluginTestIdPrefix ? { pluginTestIdPrefix } : {})}
    />,
  );
  return { onNavigate, ...view };
}

describe("DestinationRows", () => {
  afterEach(() => cleanup());

  it("renders a labelled link per destination", () => {
    renderRows([STATS, PLUGIN_PAGE]);

    expect(screen.getByRole("link", { name: "Stats" }).getAttribute("href")).toBe("/stats");
    expect(screen.getByRole("link", { name: "Hello" }).getAttribute("href")).toBe("/plugins/hello");
  });

  it("renders nothing for an empty list", () => {
    const { container } = renderRows([]);

    expect(container.innerHTML).toBe("");
  });

  it("applies the plugin test-id prefix to plugin entries only", () => {
    renderRows([STATS, PLUGIN_PAGE], "mobile-plugin-nav-item-");

    expect(screen.getByTestId("mobile-plugin-nav-item-hello")).toBeTruthy();
    expect(screen.queryByTestId("mobile-plugin-nav-item-stats")).toBeNull();
  });

  it("closes the surrounding sheet when a destination is opened", () => {
    const { onNavigate } = renderRows([STATS]);

    fireEvent.click(screen.getByRole("link", { name: "Stats" }));

    expect(onNavigate).toHaveBeenCalledTimes(1);
  });
});
