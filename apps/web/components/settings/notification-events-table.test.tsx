import { cleanup, render, screen } from "@testing-library/react";
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

function renderTable(tableEvents: string[], providers = [provider()]) {
  return render(
    <NotificationEventsTable
      tableProviders={providers}
      baselineProviders={providers}
      tableEvents={tableEvents}
      onToggleEvent={vi.fn()}
      onTestProvider={vi.fn()}
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

  it("reports an empty provider list instead of an empty table", () => {
    renderTable([NOTIFICATION_EVENT_SESSION_TURN_FINISHED], []);

    expect(screen.getByText("No providers configured yet.")).toBeTruthy();
    expect(screen.queryByTestId("notification-events-desktop-table")).toBeNull();
  });
});
