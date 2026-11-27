import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
const data = vi.hoisted(() => ({ metadata: undefined as Record<string, unknown> | undefined }));
vi.mock("@/components/state-provider", () => ({
  useOptionalAppStore: (selector: (state: unknown) => unknown) =>
    selector({ taskSessions: { items: { session: { metadata: data.metadata } } } }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, options?: { first: number; last: number }) =>
      options ? `${key}: ${options.first}..${options.last}` : key,
  }),
}));
import { DeliveryHistoryNotice } from "./delivery-history-notice";
afterEach(() => {
  cleanup();
  data.metadata = undefined;
});
describe("restored delivery history", () => {
  it("remains visible after the current recovery belongs to a new prompt", () => {
    data.metadata = {
      agent_delivery_recovery: { phase: "reconnecting" },
      agent_delivery_history_gaps: {
        stream: {
          stream_id: "stream",
          incarnation_id: "inc",
          harness_generation: 1,
          submission_id: "old-prompt",
          payload_hash: "hash",
          native_session_id: "native",
          gap: { first_sequence: 2, last_sequence: 6, received_sequence: 1, projected_sequence: 1 },
        },
      },
    };
    render(<DeliveryHistoryNotice sessionId="session" />);
    expect(screen.getByText("task:deliveryHistoryGapArchived")).toBeTruthy();
    const details = screen.getByTestId("delivery-history-details");
    expect(details.hasAttribute("open")).toBe(false);
    fireEvent.click(screen.getByTestId("delivery-history-details-summary"));
    expect(details.textContent).toContain("stream");
    expect(details.textContent).toContain("task:deliveryHistoryGapRange: 2..6");
  });
  it("does not claim unavailable output for ordinary retained replay or an unrestored candidate", () => {
    data.metadata = {
      agent_delivery_recovery: { history_gap: { first_sequence: 2, last_sequence: 6 } },
    };
    render(<DeliveryHistoryNotice sessionId="session" />);
    expect(screen.queryByRole("note")).toBeNull();
  });
});
