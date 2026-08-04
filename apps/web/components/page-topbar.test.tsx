import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageTopbar } from "./page-topbar";

vi.mock("@/components/app-status-bar/app-status-surface-provider", () => ({
  AppStatusDrawerTrigger: () => null,
}));

const PHONE_HOME = "topbar-phone-home";

describe("PageTopbar home crumb", () => {
  afterEach(cleanup);

  it("renders a phone-only home crumb by default", () => {
    render(<PageTopbar title="Hello E2E" />);

    const home = screen.getByTestId(PHONE_HOME);
    expect(home).not.toBeNull();
    // Hidden from md up, where the always-visible AppSidebar owns Home.
    expect(home.className).toContain("md:hidden");
    expect(home.querySelector("a")?.getAttribute("href")).toBe("/?home=overview");
  });

  it("keeps the crumb visible at every width for homeAffordance=always", () => {
    render(<PageTopbar title="Settings" homeAffordance="always" />);

    const home = screen.getByTestId(PHONE_HOME);
    expect(home.className).not.toContain("md:hidden");
  });

  it("suppresses the crumb for homeAffordance=none", () => {
    render(<PageTopbar title="Board" homeAffordance="none" />);

    expect(screen.queryByTestId(PHONE_HOME)).toBeNull();
  });

  it("points the crumb at homeHref when provided", () => {
    render(<PageTopbar title="Office" homeHref="/office" />);

    const home = screen.getByTestId(PHONE_HOME);
    expect(home.querySelector("a")?.getAttribute("href")).toBe("/office");
  });

  it("still renders the crumb when the page supplies leading content", () => {
    // The crumb used to be tied to `leading`; the affordance prop owns it now.
    render(<PageTopbar title="Plugins" leading={<button type="button">Open menu</button>} />);

    expect(screen.getByTestId(PHONE_HOME)).not.toBeNull();
  });

  it("omits it when the page already shows a real back link", () => {
    render(<PageTopbar title="Agent" backHref="/settings/agents" backLabel="Agents" />);

    expect(screen.queryByTestId(PHONE_HOME)).toBeNull();
    expect(screen.getByText("Agents")).not.toBeNull();
  });

  it("omits it for the root variant, which renders a plain label instead of a breadcrumb", () => {
    render(<PageTopbar title="Home" variant="root" backLabel="Kandev" />);

    expect(screen.queryByTestId(PHONE_HOME)).toBeNull();
  });
});
