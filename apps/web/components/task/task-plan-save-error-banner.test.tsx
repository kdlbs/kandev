import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { TaskPlanSaveErrorBanner } from "./task-plan-save-error-banner";

describe("TaskPlanSaveErrorBanner", () => {
  afterEach(() => {
    cleanup();
  });

  it("renders the localized size-rejection copy with the limit and submitted size (AC-003.1)", () => {
    render(
      <TaskPlanSaveErrorBanner
        saveError={{ kind: "content-too-large", limit: 262144, submitted: 300000 }}
      />,
    );
    const banner = screen.getByTestId("plan-save-error-banner");
    expect(banner.textContent).toContain("256.0 KB");
    expect(banner.textContent).toContain("293.0 KB");
  });

  it("renders the localized generic failure copy for a generic rejection, not err.message (AC-003.5/003.6)", () => {
    const backendMessage = "raw backend transport failure text that must never render";
    render(<TaskPlanSaveErrorBanner saveError={{ kind: "generic", message: backendMessage }} />);
    const banner = screen.getByTestId("plan-save-error-banner");
    expect(banner.textContent).not.toContain(backendMessage);
    // Asserts the actual translated string, not just "something rendered" —
    // the latter would pass against a raw, untranslated i18n key just as
    // easily as against real copy, which is exactly what AC-003.6 forbids.
    expect(banner.textContent).toContain("Failed to save plan");
  });

  it("keeps the limit and submitted size visually distinct when they round to the same display bucket (AC-003.1)", () => {
    // 262,150 is only 6 bytes over the 262,144 ceiling — formatBytes' one
    // decimal place rounds both to "256.0 KB", which would render as "Your
    // plan is 256.0 KB, over the 256.0 KB limit." and hide the very fact the
    // banner exists to show.
    render(
      <TaskPlanSaveErrorBanner
        saveError={{ kind: "content-too-large", limit: 262144, submitted: 262150 }}
      />,
    );
    const banner = screen.getByTestId("plan-save-error-banner");
    expect(banner.textContent).toContain("262144 B");
    expect(banner.textContent).toContain("262150 B");
  });
});
