import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type {
  DreamDetail,
  DreamSummary,
  OutcomeMeasure,
} from "@/lib/api/domains/coordinator-learning-api";

const api = vi.hoisted(() => ({
  getLearning: vi.fn(),
  putShadowDream: vi.fn(),
  getOutcomeMeasures: vi.fn(),
  listDreams: vi.fn(),
  getDream: vi.fn(),
  putDreamRating: vi.fn(),
}));
vi.mock("@/lib/api/domains/coordinator-learning-api", () => api);

import { LearningSection } from "./learning-section";

const measure = (over: Partial<OutcomeMeasure> = {}): OutcomeMeasure => ({
  value: 0.78,
  numerator: 39,
  denominator: 50,
  capped: false,
  ...over,
});

function summary(over: Partial<DreamSummary> = {}): DreamSummary {
  return {
    id: "d1",
    status: "partial",
    window_start: "2026-09-22T00:00:00Z",
    window_end: "2026-09-29T00:00:00Z",
    turn_count: 22,
    item_count: 2,
    cost_subcents: null,
    started_at: "2026-09-29T01:00:00Z",
    finished_at: "2026-09-29T01:10:00Z",
    ...over,
  };
}

function detail(): DreamDetail {
  const item = {
    position: 1,
    target_id: "",
    cited_turn_ids: ["T-118", "T-131"],
    gate: "pass",
  };
  return {
    ...summary(),
    considered: ["Raise the ceiling"],
    items: [
      { ...item, id: "i1", kind: "note_add", text: "Ask before proposing" },
      {
        ...item,
        id: "i2",
        position: 2,
        kind: "note_retire",
        text: "Old note",
        gate: "thin_evidence",
      },
    ],
  };
}

beforeEach(() => {
  api.getLearning.mockResolvedValue({
    shadow_dream: true,
    health: { state: "waiting", condition: "autonomy_off" },
  });
  api.getOutcomeMeasures.mockResolvedValue({
    days: 30,
    approval_without_edit: measure(),
    override_recurrence: measure({ value: null, null_reason: "no_data" }),
    dollars_per_merged_task: measure({ value: 4.1, denominator: 12 }),
    median_wait_seconds: measure({ value: 7800, denominator: 50 }),
    agreement: measure({ value: null, null_reason: "too_few" }),
  });
  api.listDreams.mockResolvedValue({ dreams: [summary()] });
  api.getDream.mockResolvedValue(detail());
  api.putDreamRating.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

const render31 = (canManage = true) =>
  render(<LearningSection workspaceId="w1" coordinatorId="c1" canManage={canManage} />);

describe("LearningSection", () => {
  it("shows the health condition with its fix and the five measures", async () => {
    render31();
    expect((await screen.findByTestId("learning-health-condition")).textContent).toContain(
      "Autonomy is off. Turn autonomy on.",
    );
    await screen.findByTestId("learning-measure-agreement");
    expect(screen.getByTestId("learning-measure-approval_without_edit").textContent).toContain(
      "78% (39 of 50)",
    );
    expect(screen.getByTestId("learning-measure-override_recurrence").textContent).toContain(
      "Not enough data yet",
    );
    expect(screen.getByTestId("learning-measure-median_wait_seconds").textContent).toContain(
      "2h 10m",
    );
  });

  it("lists reports and shows an unknown cost as unknown", async () => {
    render31();
    const row = await screen.findByTestId("learning-report-d1");
    expect(row.textContent).toContain("partial");
    expect(row.textContent).toContain("Cost unknown");
    expect(row.textContent).toContain("22 turns");
  });

  it("shows the empty state", async () => {
    api.listDreams.mockResolvedValue({ dreams: [] });
    render31();
    expect(await screen.findByTestId("learning-reports-empty")).toBeTruthy();
  });

  it("shows a failed reports read with a retry", async () => {
    api.listDreams.mockRejectedValueOnce(new Error("boom"));
    render31();
    fireEvent.click(
      within(await screen.findByTestId("learning-reports-error")).getByRole("button"),
    );
    expect(await screen.findByTestId("learning-report-d1")).toBeTruthy();
  });

  it("shows a failed learning read instead of the section", async () => {
    api.getLearning.mockRejectedValue(new Error("boom"));
    render31();
    expect(await screen.findByTestId("learning-error")).toBeTruthy();
    expect(screen.queryByTestId("learning-section")).toBeNull();
  });

  it("pages older reports with the cursor", async () => {
    api.listDreams
      .mockResolvedValueOnce({ dreams: [summary()], next_before: "cur" })
      .mockResolvedValueOnce({ dreams: [summary({ id: "d0" })] });
    render31();
    fireEvent.click(await screen.findByTestId("learning-reports-more"));
    expect(await screen.findByTestId("learning-report-d0")).toBeTruthy();
    expect(api.listDreams).toHaveBeenLastCalledWith("w1", "c1", "cur");
    expect(screen.queryByTestId("learning-reports-more")).toBeNull();
  });

  it("puts the switch and shows the answer", async () => {
    api.putShadowDream.mockResolvedValue({ shadow_dream: false, health: { state: "off" } });
    render31();
    fireEvent.click(await screen.findByTestId("learning-shadow-toggle"));
    await waitFor(() => expect(api.putShadowDream).toHaveBeenCalledWith("w1", "c1", false));
    expect(await screen.findByText("Off")).toBeTruthy();
  });

  it("leaves a reader without the switch and the rating", async () => {
    render31(false);
    expect(await screen.findByTestId("learning-shadow-toggle")).toHaveProperty("disabled", true);
    fireEvent.click(await screen.findByTestId("learning-report-open-d1"));
    expect(await screen.findByTestId("learning-rate-i1-useful")).toHaveProperty("disabled", true);
  });
});

describe("DreamReportDetail", () => {
  async function open() {
    render31();
    fireEvent.click(await screen.findByTestId("learning-report-open-d1"));
    await screen.findByTestId("learning-detail-header");
  }

  it("shows the gate, the considered list as unverified, and retirements", async () => {
    await open();
    expect(screen.getByTestId("learning-item-i2").textContent).toContain(
      "Gate: refused (thin_evidence)",
    );
    expect(screen.getByTestId("learning-considered").textContent).toContain(
      "Reported by the agent, not verified",
    );
    expect(screen.getByTestId("learning-considered").textContent).toContain("Raise the ceiling");
    expect(screen.getByTestId("learning-retire").textContent).toContain("Old note");
    expect(screen.getByTestId("learning-retire").textContent).not.toContain("Ask before proposing");
  });

  it("rates an item and marks the choice", async () => {
    await open();
    fireEvent.click(screen.getByTestId("learning-rate-i1-harmful"));
    await waitFor(() =>
      expect(api.putDreamRating).toHaveBeenCalledWith(
        { workspaceId: "w1", coordinatorId: "c1", dreamId: "d1", itemId: "i1" },
        "harmful",
      ),
    );
    await waitFor(() =>
      expect(screen.getByTestId("learning-rate-i1-harmful").getAttribute("aria-checked")).toBe(
        "true",
      ),
    );
  });

  it("keeps the previous rating and says so when the save fails", async () => {
    api.putDreamRating.mockRejectedValue(new Error("boom"));
    await open();
    fireEvent.click(screen.getByTestId("learning-rate-i1-useful"));
    expect(await screen.findByTestId("learning-rating-error")).toBeTruthy();
    expect(screen.getByTestId("learning-rate-i1-useful").getAttribute("aria-checked")).toBe(
      "false",
    );
  });

  it("goes back to the list", async () => {
    await open();
    fireEvent.click(screen.getByTestId("learning-detail-back"));
    expect(await screen.findByTestId("learning-section")).toBeTruthy();
  });
});
