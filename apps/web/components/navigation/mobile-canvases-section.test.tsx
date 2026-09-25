import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { MobileCanvasesSection } from "./mobile-canvases-section";

vi.mock("@/hooks/domains/features/use-feature", () => ({ useFeature: () => true }));
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(cleanup);

it("keeps the disclosure target present and hidden until expanded", () => {
  render(
    <MobileCanvasesSection
      workspaceId="workspace"
      entries={[]}
      loading={false}
      error={null}
      onRetry={() => {}}
      onNavigate={() => {}}
    />,
  );
  const toggle = screen.getByRole("button", { name: "Canvases" });
  const panelId = toggle.getAttribute("aria-controls");
  expect(panelId).toBeTruthy();
  const panel = document.getElementById(panelId!);
  expect(panel).not.toBeNull();
  expect(panel?.hidden).toBe(true);
  expect(screen.queryByRole("link", { name: "Open canvas settings" })).toBeNull();
  fireEvent.click(toggle);
  expect(panel?.hidden).toBe(false);
  expect(screen.getByRole("link", { name: "Open canvas settings" })).toBeTruthy();
  fireEvent.click(toggle);
  expect(document.getElementById(panelId!)).toBe(panel);
  expect(panel?.hidden).toBe(true);
});
