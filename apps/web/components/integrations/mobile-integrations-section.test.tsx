import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { defaultState } from "@/lib/state/default-state";
import { MobileIntegrationsSection } from "./integrations-menu";

let workspaceId = "one";
let github = true;
vi.mock("@/components/state-provider", () => ({
  useAppStore: (select: (state: unknown) => unknown) =>
    select({
      ...defaultState,
      workspaces: { activeId: workspaceId, items: [] },
    }),
}));
vi.mock("@/hooks/use-nav-availability", () => ({
  useNavAvailability: () => ({ github }),
}));
vi.mock("@/lib/routing/client-router", () => ({ usePathname: () => "/" }));
afterEach(cleanup);
beforeEach(() => {
  workspaceId = "one";
  github = true;
});

// @covers AC-UI-MOBILE-MENU-007.3
it("starts collapsed and reveals eligible links and settings on expansion", () => {
  const navigate = vi.fn();
  render(<MobileIntegrationsSection onNavigate={navigate} showSetup collapsible />);
  expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();
  const toggle = screen.getByRole("button", { name: "Integrations" });
  expect(toggle.getAttribute("aria-expanded")).toBe("false");
  fireEvent.click(toggle);
  expect(toggle.getAttribute("aria-expanded")).toBe("true");
  fireEvent.click(screen.getByRole("link", { name: "GitHub" }));
  expect(navigate).toHaveBeenCalledOnce();
  fireEvent.click(toggle);
  expect(screen.queryByTestId("mobile-integration-settings")).toBeNull();
});

// @covers AC-UI-MOBILE-MENU-007.4
it("resolves current workspace availability after changes while collapsed", () => {
  const host = render(<MobileIntegrationsSection onNavigate={() => {}} showSetup collapsible />);
  workspaceId = "two";
  github = false;
  host.rerender(<MobileIntegrationsSection onNavigate={() => {}} showSetup collapsible />);
  fireEvent.click(screen.getByRole("button", { name: "Integrations" }));
  expect(screen.queryByRole("link", { name: "GitHub" })).toBeNull();
  expect(screen.getByTestId("mobile-integration-settings").getAttribute("href")).toBe(
    "/settings/workspaces/two/integrations",
  );
});

it("retains the default always-expanded presentation for other callers", () => {
  render(<MobileIntegrationsSection onNavigate={() => {}} />);
  expect(screen.getByRole("link", { name: "GitHub" })).not.toBeNull();
  expect(screen.queryByRole("button", { name: "Integrations" })).toBeNull();
});
