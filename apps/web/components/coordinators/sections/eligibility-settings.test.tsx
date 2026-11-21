import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { EligibilitySettings } from "@/lib/api/domains/coordinator-api";
import { EligibilitySettingsGroup } from "./eligibility-settings";

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) =>
      opts ? `${key} ${JSON.stringify(opts)}` : key,
  }),
}));

const values: EligibilitySettings = {
  evidence_window_hours: 720,
  min_decisions: 20,
  unedited_floor_percent: 90,
  review_max_age_hours: 168,
  daily_limit: 10,
};

afterEach(cleanup);

describe("EligibilitySettingsGroup", () => {
  it("shows the five fields with their bounds", () => {
    render(<EligibilitySettingsGroup eligibility={values} canManage onChange={vi.fn()} />);
    for (const field of Object.keys(values)) {
      expect(screen.getByTestId(`eligibility-field-${field}`)).toBeTruthy();
    }
    expect(screen.getByTestId("eligibility-help-daily_limit").textContent).toContain(
      '"min":1,"max":100',
    );
    expect(screen.getByTestId("eligibility-containment-note")).toBeTruthy();
  });

  it("reports an edit as the whole settings object", () => {
    const onChange = vi.fn();
    render(<EligibilitySettingsGroup eligibility={values} canManage onChange={onChange} />);
    fireEvent.change(screen.getByLabelText("coordinator:eligibilityDailyLimit"), {
      target: { value: "4" },
    });
    expect(onChange).toHaveBeenCalledWith({ ...values, daily_limit: 4 });
  });

  it("marks an out-of-bounds value invalid and disables inputs without manage rights", () => {
    const { rerender } = render(
      <EligibilitySettingsGroup
        eligibility={{ ...values, daily_limit: 0 }}
        canManage
        onChange={vi.fn()}
      />,
    );
    expect(
      screen.getByLabelText("coordinator:eligibilityDailyLimit").getAttribute("aria-invalid"),
    ).toBe("true");
    rerender(
      <EligibilitySettingsGroup eligibility={values} canManage={false} onChange={vi.fn()} />,
    );
    expect(
      (screen.getByLabelText("coordinator:eligibilityDailyLimit") as HTMLInputElement).disabled,
    ).toBe(true);
  });

  it("shows a server error above the group", () => {
    render(
      <EligibilitySettingsGroup
        eligibility={values}
        canManage
        serverError="daily_limit out of range"
        onChange={vi.fn()}
      />,
    );
    expect(screen.getByTestId("eligibility-error").textContent).toBe("daily_limit out of range");
  });
});
