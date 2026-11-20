import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import type { AutomationRun } from "@/lib/types/automation";
import { MobileAutomationRow } from "./mobile-automation-row";
import i18next from "i18next";
import ptAutomations from "@/src/locales/pt-pt/automations.json";

vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ push: vi.fn() }) }));
afterEach(async () => {
  cleanup();
  await i18next.changeLanguage("en");
});
const shortcut = {
  id: "builtin:a",
  label: "Nightly audit",
  href: "/automations/a",
  available: true,
};
function run(status: AutomationRun["status"]): AutomationRun {
  return {
    id: "newest",
    automation_id: "a",
    trigger_id: "",
    trigger_type: "manual",
    task_id: "",
    status,
    dedup_key: "",
    trigger_data: {},
    error_message: "",
    created_at: "2026-11-05T10:00:00Z",
  };
}

// @covers AC-OFFICE-AUTOMATION-RUNS-001.20 AC-OFFICE-AUTOMATION-RUNS-001.22
it.each([
  {
    state: "paused",
    status: "failed",
    labels: ["Paused", "Failed"],
    color: "bg-muted-foreground/50",
  },
  { state: "running", status: "skipped", labels: ["Running", "Skipped"], color: "bg-blue-500" },
] as const)("$state and newest $status remain distinct", ({ state, status, labels, color }) => {
  render(
    <MobileAutomationRow
      shortcut={shortcut}
      activity={{ state, loading: false, error: false, lastRun: run(status) }}
      onNavigate={() => {}}
    />,
  );
  const row = screen.getByRole("link", { name: shortcut.label });
  for (const label of labels) expect(row.textContent).toContain(label);
  expect(row.querySelector(`.${color.replace("/", "\\/")}`)).not.toBeNull();
  const description = document.getElementById(row.getAttribute("aria-describedby")!);
  expect(description?.textContent).toBe(`${labels[0]}. ${labels[1]}.`);
  expect(row.textContent).not.toContain("ago");
});
it("localized long status preserves name, description and row navigation", async () => {
  const i18n = i18next;
  i18n.addResourceBundle("pt-pt", "automations", ptAutomations);
  await i18n.changeLanguage("pt-pt");
  const onNavigate = vi.fn();
  render(
    <MobileAutomationRow
      shortcut={shortcut}
      activity={{
        state: "idle",
        loading: false,
        error: false,
        lastRun: run("retry_scheduling_failed"),
      }}
      onNavigate={onNavigate}
    />,
  );
  const row = screen.getByRole("link", { name: shortcut.label });
  expect(row.textContent).toContain(ptAutomations.runStatusRetrySchedulingFailed);
  expect(row.getAttribute("href")).toBe(shortcut.href);
  expect(document.getElementById(row.getAttribute("aria-describedby")!)?.textContent).toBe(
    `${ptAutomations.runStatusRetrySchedulingFailed}.`,
  );
  fireEvent.click(row);
  expect(onNavigate).toHaveBeenCalledOnce();
});
it("unavailable catalog entries remain non-navigable", () => {
  render(
    <MobileAutomationRow shortcut={{ ...shortcut, available: false }} onNavigate={() => {}} />,
  );
  expect(screen.queryByRole("link", { name: shortcut.label })).toBeNull();
  expect(screen.getByLabelText(shortcut.label).getAttribute("aria-disabled")).toBe("true");
});
it("unknown activity retains its error description without exposing stale failure", () => {
  render(
    <MobileAutomationRow
      shortcut={shortcut}
      activity={{ state: "unknown", loading: false, error: true, lastRun: run("failed") }}
      onNavigate={() => {}}
    />,
  );
  const row = screen.getByRole("link", { name: shortcut.label });
  expect(screen.queryByText("Failed", { exact: true })).toBeNull();
  expect(document.getElementById(row.getAttribute("aria-describedby")!)?.textContent).toBe(
    "Failed to load automation activity",
  );
});
