import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { IconBrandGithub, IconList } from "@tabler/icons-react";
import type { ProjectedSidebarNode } from "@/lib/sidebar/layout-projection";
import type { ShortcutActivity } from "@/hooks/domains/sidebar/use-shortcut-activity";
import { ShortcutSection } from "./shortcut-section";

afterEach(() => cleanup());

const node: ProjectedSidebarNode = {
  id: "group-1",
  kind: "shortcuts",
  visible: true,
  name: "Pinned",
  label: "Pinned",
  icon: IconList,
  shortcuts: [
    {
      id: "github",
      target: { kind: "destination", id: "github" },
      label: "GitHub",
      icon: IconBrandGithub,
      href: "/github",
      source: "builtin",
      available: true,
    },
    {
      id: "canvas",
      target: { kind: "canvas", id: "canvas-1" },
      label: "Canvas",
      icon: IconList,
      href: "/canvases/canvas-1",
      source: "canvas",
      available: true,
    },
    {
      id: "automation",
      target: { kind: "automation", id: "automation-1" },
      label: "Nightly",
      icon: IconList,
      href: "/automations/automation-1",
      source: "automation",
      available: true,
    },
    {
      id: "plugin",
      target: { kind: "destination", id: "plugin:slack:home" },
      label: "Slack",
      icon: IconList,
      href: "/plugins/slack",
      source: "plugin",
      available: true,
    },
    {
      id: "terminal",
      target: { kind: "host_action", id: "quick_terminal" },
      label: "Quick terminal",
      icon: IconList,
      source: "host_action",
      available: true,
    },
  ],
};

const activity: ShortcutActivity = { state: "running", loading: false, error: false };

describe("ShortcutSection", () => {
  beforeEach(() => onActivateShortcutMock.mockReset());

  it("uses one ordered shortcut collection for header actions and expanded rows", () => {
    render(
      <ShortcutSection
        node={node}
        mobile
        getActivity={(id) => (id === "automation-1" ? activity : undefined)}
        onActivateShortcut={() => undefined}
      />,
    );

    expect(screen.getByRole("link", { name: "GitHub" }).getAttribute("href")).toBe("/github");
    expect(screen.getByRole("link", { name: "Canvas" }).getAttribute("href")).toBe(
      "/canvases/canvas-1",
    );
    expect(screen.getByRole("link", { name: "Nightly" }).getAttribute("href")).toBe(
      "/automations/automation-1",
    );

    fireEvent.click(screen.getByRole("button", { name: "Pinned" }));
    expect(screen.getByTestId("shortcut-section-rows").getAttribute("hidden")).toBeNull();
    expect(screen.getAllByText("Nightly").length).toBeGreaterThan(0);
  });

  it("keeps overflow and host actions reachable without turning the section into a link", () => {
    const onActivateShortcut = (shortcut: { id: string }) => {
      if (shortcut.id === "terminal") onActivateShortcutMock();
    };
    render(
      <ShortcutSection
        node={node}
        mobile
        getActivity={() => undefined}
        onActivateShortcut={onActivateShortcut}
      />,
    );

    const more = screen.getByRole("button", { name: "Show more actions" });
    fireEvent.pointerDown(more);
    expect(screen.getByRole("menuitem", { name: "Quick terminal" })).toBeTruthy();
    fireEvent.click(screen.getByRole("menuitem", { name: "Quick terminal" }));
    expect(onActivateShortcutMock).toHaveBeenCalledOnce();
  });

  it("describes running activity on the mobile section trigger", () => {
    render(
      <ShortcutSection
        node={node}
        mobile
        getActivity={(id) => (id === "automation-1" ? activity : undefined)}
      />,
    );

    const trigger = screen.getByRole("button", { name: "Pinned" });
    const descriptionId = trigger.getAttribute("aria-describedby");
    expect(descriptionId).toBeTruthy();
    expect(document.getElementById(descriptionId ?? "")?.textContent).toContain("Running");
  });
});

const onActivateShortcutMock = vi.fn();
