import { act, cleanup, render, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { WalkthroughGenerationTime } from "./walkthrough-generation-time";

function expectedDate(value: string, locale = "en", withTimezone = false) {
  return new Intl.DateTimeFormat(locale, {
    year: "numeric",
    month: "short",
    day: "numeric",
    hour: "numeric",
    minute: "2-digit",
    ...(withTimezone ? { timeZoneName: "short" as const } : {}),
  }).format(new Date(value));
}

const SAVED_AT = "2026-10-28T10:42:00.123456789Z";

beforeEach(async () => {
  await activateLocale("en");
});

afterEach(async () => {
  cleanup();
  await activateLocale("en");
});

describe("walkthrough generation time", () => {
  // @covers AC-TASKS-WALKTHROUGH-TIME-001.1
  it("renders the saved time with a semantic datetime", () => {
    render(<WalkthroughGenerationTime updatedAt={SAVED_AT} />);
    const time = screen.getByText(/Generated/);
    expect(time.tagName).toBe("TIME");
    expect(time.getAttribute("datetime")).toBe(SAVED_AT);
    expect(time.textContent).toBe(`Generated ${expectedDate(SAVED_AT)}`);
    expect(time.getAttribute("title")).toBe(`Generated ${expectedDate(SAVED_AT, "en", true)}`);
  });

  // @covers AC-TASKS-WALKTHROUGH-TIME-001.2
  it("updates the saved time on rerender", () => {
    const { rerender } = render(<WalkthroughGenerationTime updatedAt={SAVED_AT} />);
    const replacement = "2026-10-29T15:16:00Z";
    rerender(<WalkthroughGenerationTime updatedAt={replacement} />);
    expect(screen.getByText(/Generated/).getAttribute("datetime")).toBe(replacement);
    expect(screen.getByText(/Generated/).textContent).toBe(
      `Generated ${expectedDate(replacement)}`,
    );
  });

  // @covers AC-TASKS-WALKTHROUGH-TIME-001.3
  it("reformats the same instant after a language change", async () => {
    render(<WalkthroughGenerationTime updatedAt={SAVED_AT} />);
    await act(async () => {
      await activateLocale("pt-pt");
    });
    const time = screen.getByText(/Gerado/);
    expect(time.getAttribute("datetime")).toBe(SAVED_AT);
    expect(time.textContent).toBe(`Gerado em ${expectedDate(SAVED_AT, "pt-pt")}`);
    expect(time.getAttribute("title")).toBe(`Gerado em ${expectedDate(SAVED_AT, "pt-pt", true)}`);
  });

  // @covers AC-TASKS-WALKTHROUGH-TIME-001.5
  it.each([undefined, "", " ", "not-a-date", "0", "2026-02-30T10:42:00Z"])(
    "omits missing and invalid generation times: %j",
    (updatedAt) => {
      const { container } = render(<WalkthroughGenerationTime updatedAt={updatedAt} />);
      expect(container.textContent).toBe("");
      expect(container.querySelector("time")).toBeNull();
    },
  );
});
