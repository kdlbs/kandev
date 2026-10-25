import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { TooltipProvider } from "@kandev/ui/tooltip";

import { ScheduleSelector, isValidExpression } from "./schedule-selector";

const NEXT_RUN = "schedule-next-run";
const ADOPT_TZ = "schedule-adopt-timezone";
const FREQUENCY_SELECTOR = "schedule-frequency";
const CUSTOM_INPUT = "schedule-custom-input";
const CUSTOM_WEEKDAYS = "0 9 * * 1-5";
const DAILY_9 = "0 9 * * *";

function renderSelector(config: Record<string, unknown> | null) {
  const onChange = vi.fn();
  render(
    <TooltipProvider>
      <ScheduleSelector config={config} onChange={onChange} />
    </TooltipProvider>,
  );
  return onChange;
}

afterEach(cleanup);

describe("ScheduleSelector preview", () => {
  it("names the resolved instant in both the chosen zone and UTC", () => {
    // The whole point of the line: a cron alone never says which moment it
    // means, and the two readings differ for any zone that is not UTC.
    renderSelector({ cron_expression: DAILY_9, timezone: "Asia/Singapore" });

    const preview = screen.getByTestId(NEXT_RUN);
    expect(preview.textContent).toContain("09:00");
    expect(preview.textContent).toContain("GMT+8");
    expect(preview.textContent).toContain("UTC");
  });

  it("shows no instant for an interval, which the server anchors", () => {
    renderSelector({ cron_expression: "@every 15m" });
    expect(screen.queryByTestId(NEXT_RUN)).toBeNull();
  });

  it("hides the timezone control for an interval", () => {
    renderSelector({ cron_expression: "@every 15m" });
    expect(screen.queryByTestId("schedule-timezone")).toBeNull();
  });

  it("promises nothing until a schedule is actually saved", () => {
    // The controls fall back to a sensible default, but no trigger exists yet
    // and the scheduler skips an empty expression — so there is no next run to
    // announce.
    renderSelector({ cron_expression: "" });
    expect(screen.getByTestId(NEXT_RUN).textContent).toContain("won't run on its own");
  });

  it("promises nothing for an automation with no schedule at all", () => {
    renderSelector(null);
    expect(screen.getByTestId(NEXT_RUN).textContent).toContain("won't run on its own");
  });

  it("shows the unscheduled state and lets the user create a daily schedule", () => {
    const onChange = renderSelector(null);

    expect(screen.getByTestId(FREQUENCY_SELECTOR).textContent).toBe("No schedule");

    fireEvent.click(screen.getByTestId(FREQUENCY_SELECTOR));
    fireEvent.click(screen.getByRole("option", { name: "every day" }));

    expect(onChange).toHaveBeenCalledWith(expect.objectContaining({ cron_expression: DAILY_9 }));
  });

  it("opens the custom editor from the unscheduled state", () => {
    const onChange = renderSelector(null);

    fireEvent.click(screen.getByTestId(FREQUENCY_SELECTOR));
    fireEvent.click(screen.getByRole("option", { name: "a custom schedule" }));

    const input = screen.getByTestId(CUSTOM_INPUT);
    fireEvent.change(input, { target: { value: "@every 2h" } });
    fireEvent.blur(input);

    expect(onChange).toHaveBeenCalledWith({ cron_expression: "@every 2h", timezone: undefined });
  });
});

describe("ScheduleSelector timezone migration", () => {
  it("leaves a saved schedule without a timezone on UTC", () => {
    // Relocalising it would move an automation that is already running by the
    // whole offset, with nothing on screen to say so.
    renderSelector({ cron_expression: DAILY_9 });

    expect(screen.getByTestId("schedule-timezone").textContent).toContain("UTC");
    expect(screen.getByTestId(ADOPT_TZ)).toBeTruthy();
  });

  it("adopts a real timezone only when asked", () => {
    const onChange = renderSelector({ cron_expression: DAILY_9 });

    fireEvent.click(screen.getByTestId(ADOPT_TZ));

    const emitted = onChange.mock.calls[0][0];
    expect(emitted.cron_expression).toBe(DAILY_9);
    expect(typeof emitted.timezone).toBe("string");
    expect(emitted.timezone).not.toBe("");
  });

  it("does not offer the prompt once a timezone is set", () => {
    renderSelector({ cron_expression: DAILY_9, timezone: "Asia/Singapore" });
    expect(screen.queryByTestId(ADOPT_TZ)).toBeNull();
  });

  it("gives a brand new schedule the viewer's own timezone", () => {
    const onChange = renderSelector(null);

    fireEvent.click(screen.getByTestId(FREQUENCY_SELECTOR));
    fireEvent.click(screen.getByRole("option", { name: "every day" }));

    const emitted = onChange.mock.calls[0][0];
    expect(emitted.cron_expression).toBe(DAILY_9);
    expect(emitted.timezone).toBeTruthy();
    expect(screen.queryByTestId(ADOPT_TZ)).toBeNull();
  });
});

describe("ScheduleSelector custom expressions", () => {
  it("opens an unrepresentable cron in the custom field, verbatim", () => {
    renderSelector({ cron_expression: CUSTOM_WEEKDAYS, timezone: "UTC" });
    expect(screen.getByTestId(CUSTOM_INPUT).getAttribute("value")).toBe(CUSTOM_WEEKDAYS);
  });

  it("rejects an unreadable expression without emitting", () => {
    const onChange = renderSelector({ cron_expression: CUSTOM_WEEKDAYS, timezone: "UTC" });

    const input = screen.getByTestId(CUSTOM_INPUT);
    fireEvent.change(input, { target: { value: "not a cron" } });
    fireEvent.blur(input);

    expect(screen.getByTestId("schedule-error")).toBeTruthy();
    expect(onChange).not.toHaveBeenCalled();
  });

  it("accepts the grammar the backend accepts", () => {
    const onChange = renderSelector({ cron_expression: CUSTOM_WEEKDAYS, timezone: "UTC" });

    const input = screen.getByTestId(CUSTOM_INPUT);
    fireEvent.change(input, { target: { value: "*/10 8-18 * * 1-5" } });
    fireEvent.blur(input);

    expect(screen.queryByTestId("schedule-error")).toBeNull();
    expect(onChange).toHaveBeenCalledWith({
      cron_expression: "*/10 8-18 * * 1-5",
      timezone: "UTC",
    });
  });
});

describe("isValidExpression", () => {
  it("accepts what the scheduler accepts", () => {
    for (const expression of [
      "",
      "@daily",
      "@every 2h30m",
      DAILY_9,
      CUSTOM_WEEKDAYS,
      "*/10 * * * *",
      "0 9,17 * * *",
    ]) {
      expect(isValidExpression(expression), expression).toBe(true);
    }
  });

  // Verified against the scheduler's own parser (robfig/cron with Month and
  // Dow enabled): every expression below parses there, so rejecting one here
  // would block a schedule the backend would have run.
  it("accepts the named month and weekday forms the scheduler parses", () => {
    for (const expression of [
      "0 9 * * MON-FRI",
      "0 9 * * mon,wed,fri",
      "0 0 1 JAN *",
      "0 9 * JAN-MAR *",
      "0 9 * * ?",
      "0 9 ? * MON",
    ]) {
      expect(isValidExpression(expression), expression).toBe(true);
    }
  });

  it("rejects what it cannot parse", () => {
    for (const expression of [
      "nonsense",
      "0 9 * *",
      "@every",
      "0 9 * * * *",
      // Quartz-only syntax the scheduler's parser rejects, so the editor must
      // too — accepting it would save a schedule that never fires.
      "0 9 L * *",
      "0 9 * * SUN#2",
      // Names belong to the month and weekday fields alone.
      "0 MON * * *",
      "0 9 * * JAN",
      // "?" is a day-field alias; elsewhere it is not a field at all.
      "? 9 * * *",
    ]) {
      expect(isValidExpression(expression), expression).toBe(false);
    }
  });
});
