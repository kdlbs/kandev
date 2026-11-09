import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { StateProvider } from "@/components/state-provider";
import { defaultState } from "@/lib/state/default-state";
import type { QueuedMessage } from "@/lib/state/slices/session/types";
import type { MessageTimeDisplay } from "@/lib/types/http-user-settings";
import { formatMessageTime } from "@/lib/i18n/message-time";
import { QueuedGhostMessage } from "./queued-ghost-message";

const touch = vi.hoisted(() => ({ enabled: false }));
vi.mock("@/hooks/use-compact-task-chrome", () => ({ useTouchDrawer: () => touch.enabled }));
const QUEUED_AT = "2026-10-28T09:32:00Z";
const LATEST_AT = "2026-10-28T09:36:00Z";

function row(overrides: Partial<QueuedMessage> = {}, display: MessageTimeDisplay = "relative") {
  const entry: QueuedMessage = {
    id: "queue-1",
    session_id: "session-1",
    task_id: "task-1",
    content: "Queued follow-up",
    plan_mode: false,
    queued_by: "user",
    queued_at: QUEUED_AT,
    ...overrides,
  };
  return (
    <StateProvider
      initialState={{ userSettings: { ...defaultState.userSettings, messageTimeDisplay: display } }}
    >
      <QueuedGhostMessage
        entry={entry}
        canEdit={false}
        onSave={async () => {}}
        onRemove={() => {}}
      />
    </StateProvider>
  );
}

afterEach(() => {
  cleanup();
  touch.enabled = false;
});

// @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.1, AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.2
describe("queued message timestamps", () => {
  it.each(["user", "agent", "workflow", "server"])(
    "shows the queued time for %s messages",
    (queued_by) => {
      const { container } = render(row({ queued_by }));
      expect(container.querySelector("time")?.getAttribute("datetime")).toBe(QUEUED_AT);
    },
  );

  it("updates the surviving row when a merge supplies a newer server timestamp", () => {
    const view = render(row());
    expect(view.container.querySelector("time")?.getAttribute("datetime")).toBe(QUEUED_AT);
    view.rerender(row({ content: "Queued follow-up\n\nAnother follow-up", queued_at: LATEST_AT }));
    expect(view.container.querySelector("time")?.getAttribute("datetime")).toBe(LATEST_AT);
  });

  it("shows time when only an attachment is queued", () => {
    const { container } = render(
      row({
        content: "",
        attachments: [
          {
            type: "file",
            mime_type: "text/plain",
            name: "notes.txt",
            attachment_id: "attachment-1",
          },
        ],
      }),
    );
    expect(container.querySelector("time")?.getAttribute("datetime")).toBe(QUEUED_AT);
  });

  // @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.3
  it.each(["relative", "absolute_short", "absolute_long"] as const)(
    "uses the saved %s format",
    (display) => {
      const { container } = render(row({}, display));
      const formatted = formatMessageTime(QUEUED_AT, display)!;
      const time = container.querySelector("time");
      expect(time?.textContent).toBe(formatted.label);
      expect(time?.getAttribute("title")).toBe(formatted.counterpart);
      expect(time?.getAttribute("aria-label")).toContain(formatted.counterpart);
    },
  );

  // @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.4
  it.each([undefined, "", "not-a-date", "0", "2026-02-30T10:00:00Z"])(
    "omits invalid time %j without hiding the message",
    (queued_at) => {
      touch.enabled = true;
      const { container } = render(row({ queued_at }));
      expect(container.querySelector("time")).toBeNull();
      expect(screen.queryByTestId("message-timestamp-trigger")).toBeNull();
      expect(screen.getByText("Queued follow-up")).toBeTruthy();
    },
  );

  // @covers AC-UI-QUEUED-MESSAGE-TIMESTAMPS-001.5
  it("opens the counterpart details on touch", () => {
    touch.enabled = true;
    render(row());
    fireEvent.click(screen.getByTestId("message-timestamp-trigger"));
    expect(screen.getByTestId("message-timestamp-drawer").textContent).toContain(
      formatMessageTime(QUEUED_AT, "relative")!.counterpart,
    );
  });
});
