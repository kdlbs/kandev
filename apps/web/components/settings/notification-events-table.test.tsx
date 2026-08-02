import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED,
  NOTIFICATION_EVENT_SESSION_TURN_FINISHED,
} from "@/lib/notifications/events";
import type { NotificationProvider } from "@/lib/types/http";
import { NotificationEventsTable } from "./notification-events-table";

function provider(overrides: Partial<NotificationProvider> = {}): NotificationProvider {
  return {
    id: "provider-1",
    name: "Desktop Notifications",
    type: "local",
    config: {},
    enabled: true,
    events: [],
    created_at: "",
    updated_at: "",
    ...overrides,
  };
}

function renderTable(tableEvents: string[], providers = [provider()], onTestProvider = vi.fn()) {
  return render(
    <NotificationEventsTable
      tableProviders={providers}
      baselineProviders={providers}
      tableEvents={tableEvents}
      onToggleEvent={vi.fn()}
      onTestProvider={onTestProvider}
    />,
  );
}

afterEach(cleanup);

describe("NotificationEventsTable", () => {
  it("renders the translated title and description for a known event type", () => {
    renderTable([NOTIFICATION_EVENT_SESSION_TURN_FINISHED]);

    // Once in the mobile list and once in the desktop table; both are always
    // mounted and hidden by CSS, so each string appears twice.
    expect(screen.getAllByText("Agent turn finished")).toHaveLength(2);
    expect(screen.getAllByText("Notify after each completed agent turn.")).toHaveLength(2);
  });

  it("falls back to the raw event type for an event the catalog does not know", () => {
    renderTable(["session.some_future_event"]);

    expect(screen.getAllByText("session.some_future_event")).toHaveLength(2);
    expect(screen.getAllByText("Notify when this event occurs.")).toHaveLength(2);
  });

  it("names each checkbox with the event title and the provider name", () => {
    renderTable([NOTIFICATION_EVENT_SESSION_CLARIFICATION_REQUESTED]);

    expect(
      screen.getAllByRole("checkbox", {
        name: "Agent needs an answer for Desktop Notifications",
      }),
    ).toHaveLength(2);
  });

  it("names and wires the test control for a remote provider", async () => {
    const onTestProvider = vi.fn().mockResolvedValue(undefined);
    const remote = provider({ id: "apprise-1", name: "Ops channel", type: "apprise" });
    renderTable([NOTIFICATION_EVENT_SESSION_TURN_FINISHED], [remote], onTestProvider);

    const buttons = screen.getAllByRole("button", {
      name: "Send test notification for Ops channel",
    });
    expect(buttons).toHaveLength(2);

    // Radix Tooltip does not open from synthetic pointer events in this
    // environment; focus is the reliable path (apps/web/AGENTS.md).
    fireEvent.focus(buttons[0]);
    expect((await screen.findByRole("tooltip")).textContent).toBe("Send test notification");

    fireEvent.click(buttons[0]);
    expect(onTestProvider).toHaveBeenCalledWith("apprise-1");
  });

  it("omits the test control for the local provider, which cannot be tested remotely", () => {
    renderTable([NOTIFICATION_EVENT_SESSION_TURN_FINISHED]);

    expect(screen.queryByRole("button", { name: /Send test notification/ })).toBeNull();
  });

  it("reports an empty provider list instead of an empty table", () => {
    renderTable([NOTIFICATION_EVENT_SESSION_TURN_FINISHED], []);

    expect(screen.getByText("No providers configured yet.")).toBeTruthy();
    expect(screen.queryByTestId("notification-events-desktop-table")).toBeNull();
  });
});
