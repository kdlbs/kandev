import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { PageTopbar, TOPBAR_HEIGHT_CLASSNAME } from "./page-topbar";

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

  it("measures the phone-only home crumb when pressure measurement is enabled", () => {
    render(<PageTopbar title="Plugins" parents={[{ label: "Workspace", href: "/workspace" }]} />);

    for (const ghostHome of screen.getAllByTestId("topbar-ghost-home")) {
      expect(ghostHome.className).toContain("md:hidden");
    }
  });

  it("omits it when the page already shows a real back link", () => {
    render(<PageTopbar title="Agent" backHref="/settings/agents" backLabel="Agents" />);

    expect(screen.queryByTestId(PHONE_HOME)).toBeNull();
    expect(screen.getByText("Agents")).not.toBeNull();
  });
});

describe("PageTopbar parent crumbs", () => {
  afterEach(cleanup);

  const parents = [
    { label: "Settings", href: "/settings", phoneOnlyLink: true },
    { label: "Workspaces", href: "/settings/workspaces" },
    { label: "Kanban1", href: "/settings/workspaces/ws-1" },
  ];

  it("collapses all but the last parent into a phone-only overflow menu", () => {
    render(<PageTopbar title="Integrations" parents={parents} />);

    // Last parent stays visible at every width.
    const lastParent = screen.getByRole("link", { name: "Kanban1" });
    expect(lastParent.closest("li")?.className).not.toContain("max-md:hidden");

    // Earlier crumbs render for md+ only…
    const middle = screen.getByRole("link", { name: "Workspaces" });
    expect(middle.closest("li")?.className).toContain("max-md:hidden");

    // …and reappear inside the phone-only "…" dropdown trigger.
    const overflow = screen.getByTestId("topbar-crumb-overflow");
    expect(overflow.closest("li")?.className).toContain("md:hidden");
  });

  it("names the overflow trigger for assistive technology", () => {
    // `BreadcrumbEllipsis` marks itself `aria-hidden`, so its own sr-only text
    // never reaches the accessibility tree — the trigger has to be named here
    // or it announces as a bare "button".
    render(<PageTopbar title="Integrations" parents={parents} />);

    expect(screen.getByRole("button", { name: "Show more breadcrumbs" })).toBe(
      screen.getByTestId("topbar-crumb-overflow"),
    );
  });

  it("renders no overflow menu for a single parent", () => {
    render(<PageTopbar title="Workspaces" parents={[parents[0]]} />);

    expect(screen.queryByTestId("topbar-crumb-overflow")).toBeNull();
    // The lone parent is not collapsed away.
    expect(screen.getByRole("link", { name: "Settings" }).closest("li")?.className).not.toContain(
      "max-md:hidden",
    );
  });
});

describe("PageTopbar titleSlot", () => {
  afterEach(cleanup);

  it("renders the slot as the current crumb without a dead link role", () => {
    render(
      <PageTopbar
        title="Checkout revamp"
        testId="slot-topbar"
        titleSlot={<input aria-label="Rename task" defaultValue="Checkout revamp" />}
      />,
    );

    const page = screen.getByTestId("slot-topbar").querySelector('[data-slot="breadcrumb-page"]');
    // The slot renders inside the current-page crumb, not merely somewhere.
    expect(page?.contains(screen.getByLabelText("Rename task"))).toBe(true);
    expect(page?.getAttribute("aria-current")).toBe("page");
    // `BreadcrumbPage` hardcodes `role="link" aria-disabled="true"`, which would
    // wrap this interactive control in a disabled link that goes nowhere.
    expect(page?.getAttribute("role")).toBeNull();
    expect(page?.getAttribute("aria-disabled")).toBeNull();
    // The plain-text title span is replaced, not doubled.
    expect(screen.queryByText("Checkout revamp")).toBeNull();
  });

  it("keeps the disabled-link crumb semantics for a plain text title", () => {
    render(<PageTopbar title="Checkout revamp" testId="text-topbar" />);

    const page = screen.getByTestId("text-topbar").querySelector('[data-slot="breadcrumb-page"]');
    expect(page?.getAttribute("role")).toBe("link");
    expect(page?.getAttribute("aria-disabled")).toBe("true");
    expect(page?.getAttribute("aria-current")).toBe("page");
    expect(page?.textContent).toBe("Checkout revamp");
  });
});

describe("PageTopbar overflow actions", () => {
  afterEach(cleanup);

  it("renders overflow actions inline while there is room", () => {
    render(
      <PageTopbar
        title="Task"
        testId="overflow-topbar"
        overflowActions={<button type="button">Archive</button>}
      />,
    );

    // Inline means inside this header's action row, not merely mounted.
    const header = screen.getByTestId("overflow-topbar");
    expect(header.contains(screen.getByRole("button", { name: "Archive" }))).toBe(true);
    // Without pressure (no layout in the test DOM) the fold never engages.
    expect(screen.queryByTestId("topbar-actions-overflow")).toBeNull();
  });
});

describe("PageTopbar free width", () => {
  afterEach(cleanup);

  function zones(testId: string) {
    const header = screen.getByTestId(testId);
    // Lead zone first, trailing zone last: the ghost row is absolute and the
    // center is only rendered when a bar asks for one.
    const flexZones = Array.from(header.children).filter((el) =>
      el.className.includes("items-center gap-3"),
    );
    return { lead: flexZones[0], right: flexZones[flexZones.length - 1] };
  }

  it("gives the slack to the lead zone by default", () => {
    render(
      <PageTopbar title="Tasks" testId="lead-slack" actions={<button type="button">A</button>} />,
    );

    const { lead, right } = zones("lead-slack");
    expect(lead?.className).toContain("grow");
    // A default action cluster is `shrink-0`, so the zone stays at full width
    // and every shrink lands on the lead side.
    expect(right?.className).toContain("shrink");
    expect(right?.className).not.toContain("grow");
  });

  it("hands the slack to the actions when the cluster is the flexible one", () => {
    render(
      <PageTopbar
        title="Tasks"
        testId="actions-slack"
        freeWidth="actions"
        actionsClassName="min-w-0 flex-1 !shrink"
        actions={<button type="button">A</button>}
      />,
    );

    const { lead, right } = zones("actions-slack");
    // Without this the `flex-1` cluster resolves to zero width — its base size
    // is 0 and a shrink-only zone never grows to give it any.
    expect(right?.className).toContain("grow");
    expect(right?.className).toContain("min-w-0");
    // And the lead zone must stop spreading, or it takes the slack first.
    expect(lead?.className).not.toContain("grow");
  });
});

describe("PageTopbar height token", () => {
  afterEach(cleanup);

  it("applies the shared height token to the header", () => {
    render(<PageTopbar title="Stats" testId="stats-topbar" />);

    const header = screen.getByTestId("stats-topbar");
    for (const token of TOPBAR_HEIGHT_CLASSNAME.split(" ")) {
      expect(header.className).toContain(token);
    }
  });
});
