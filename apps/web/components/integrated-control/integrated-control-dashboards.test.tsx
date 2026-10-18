import { act, cleanup, render, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { StateProvider } from "@/components/state-provider";
import { activateLocale } from "@/lib/i18n";
import { defaultFeatureFlags } from "@/lib/state/slices/features/types";
import { IntegratedControlProvider } from "./integrated-control-provider";
import { IntegratedControlDashboard } from "./integrated-control-dashboards";

const api = vi.hoisted(() => ({ workspaces: vi.fn(), coordinators: vi.fn() }));
vi.mock("@/lib/api", async (original) => ({
  ...(await original<typeof import("@/lib/api")>()),
  listWorkspaces: api.workspaces,
}));
vi.mock("@/lib/api/domains/coordinator-api", () => ({ listCoordinators: api.coordinators }));
vi.mock("@/lib/ws/connection", () => ({ useWebSocketClient: () => null }));
const updated = "2026-07-27T12:34:00Z";
beforeEach(() => {
  window.history.replaceState({}, "", "/control/activity");
  api.workspaces
    .mockReset()
    .mockResolvedValue({ workspaces: [{ id: "beta", name: "Beta", scopes: ["workspace.read"] }] });
  api.coordinators.mockReset().mockResolvedValue({
    coordinators: [
      { id: "coordinator", workspace_id: "beta", name: "Coordinator", updated_at: updated },
    ],
  });
});
afterEach(async () => {
  cleanup();
  await activateLocale("en");
  window.history.replaceState({}, "", "/");
});

it("renders the activity timestamp with the active locale and preserves its machine value", async () => {
  await activateLocale("pt-pt");
  const view = render(
    <StateProvider
      initialState={{
        features: { ...defaultFeatureFlags, integratedControl: true, coordinator: true },
      }}
    >
      <IntegratedControlProvider>
        <IntegratedControlDashboard view="activity" />
      </IntegratedControlProvider>
    </StateProvider>,
  );
  await waitFor(() => expect(view.container.querySelector("time")).not.toBeNull());
  const expected = (locale: string) =>
    new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(
      new Date(updated),
    );
  expect(view.container.querySelector("time")?.getAttribute("datetime")).toBe(updated);
  expect(view.container.querySelector("time")?.textContent).toBe(expected("pt-PT"));
  await act(async () => {
    await activateLocale("ja");
  });
  expect(view.container.querySelector("time")?.textContent).toBe(expected("ja"));
});
