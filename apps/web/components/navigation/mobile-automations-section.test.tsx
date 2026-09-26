import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { MobileAutomationsSection } from "./mobile-automations-section";
const api = vi.hoisted(() => ({ listAutomations: vi.fn(), listAutomationSummaries: vi.fn() }));
vi.mock("@/lib/api/domains/automation-api", () => api);
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
const automation = {
  id: "a",
  workspace_id: "one",
  name: "Daily check",
  enabled: true,
  max_concurrent_runs: 1,
  triggers: [],
};
beforeEach(() => {
  api.listAutomations.mockReset().mockResolvedValue([automation]);
  api.listAutomationSummaries.mockReset().mockResolvedValue([]);
});
afterEach(cleanup);
function open() {
  fireEvent.click(screen.getByRole("button", { name: "Automations" }));
}
it("defers reads until opened and exposes the existing detail route", async () => {
  render(<MobileAutomationsSection workspaceId="one" onNavigate={() => {}} />);
  expect(api.listAutomations).not.toHaveBeenCalled();
  open();
  expect((await screen.findByRole("link", { name: /Daily check/ })).getAttribute("href")).toBe(
    "/automations/a",
  );
});
it("reports a failed list and recovers without calling it empty", async () => {
  api.listAutomations.mockRejectedValueOnce(new Error("private detail"));
  render(<MobileAutomationsSection workspaceId="one" onNavigate={() => {}} />);
  open();
  expect((await screen.findByRole("alert")).textContent).not.toContain("private detail");
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  await screen.findByRole("link", { name: /Daily check/ });
  expect(screen.queryByRole("alert")).toBeNull();
});
it("hides old workspace names while the replacement read is pending", async () => {
  const view = render(<MobileAutomationsSection workspaceId="one" onNavigate={() => {}} />);
  open();
  await screen.findByRole("link", { name: /Daily check/ });
  api.listAutomations.mockImplementation(() => new Promise(() => {}));
  view.rerender(<MobileAutomationsSection workspaceId="two" onNavigate={() => {}} />);
  expect(screen.queryByRole("link", { name: /Daily check/ })).toBeNull();
  await waitFor(() => expect(api.listAutomations).toHaveBeenLastCalledWith("two"));
  expect(screen.getByRole("status").textContent).toContain("Loading");
});
it("does not report a healthy status when activity failed", async () => {
  api.listAutomationSummaries.mockRejectedValueOnce(new Error("private activity"));
  render(<MobileAutomationsSection workspaceId="one" onNavigate={() => {}} />);
  open();
  const row = await screen.findByRole("link", { name: /Daily check/ });
  await screen.findByRole("alert");
  expect(row.textContent).not.toContain("Idle");
  fireEvent.click(screen.getByRole("button", { name: "Try again" }));
  await waitFor(() => expect(screen.queryByRole("alert")).toBeNull());
});
