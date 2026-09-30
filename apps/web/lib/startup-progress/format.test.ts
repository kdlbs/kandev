import type { TFunction } from "i18next";
import i18next from "i18next";
import { afterEach, describe, expect, it } from "vitest";
import {
  formatDuration,
  formatElapsed,
  formatEta,
  formatPhaseLabel,
  formatStalled,
  formatStepLabel,
  formatStepProgress,
  percentDone,
} from "./format";
import type { StartupStepSnapshot } from "./types";

const t = i18next.t.bind(i18next) as TFunction;

afterEach(async () => {
  await i18next.changeLanguage("en");
});

function step(overrides: Partial<StartupStepSnapshot>): StartupStepSnapshot {
  return {
    id: "stores.repositories",
    label_key: "startup.step.stores_repositories",
    measure: "counted",
    unit: "turns",
    elapsed_ms: 0,
    stalled: false,
    ...overrides,
  };
}

describe("formatDuration", () => {
  it("renders singular seconds", () => {
    expect(formatDuration(t, 1000)).toBe("1 second");
  });

  it("renders plural seconds", () => {
    expect(formatDuration(t, 2000)).toBe("2 seconds");
  });

  it("renders minutes past the 60s boundary", () => {
    expect(formatDuration(t, 60001)).toBe("2 minutes");
  });
});

describe("formatElapsed", () => {
  it("wraps the duration in the elapsed template", () => {
    expect(formatElapsed(t, 5000)).toBe("5 seconds elapsed");
  });
});

describe("formatEta", () => {
  it("wraps the duration in the eta template", () => {
    expect(formatEta(t, 30000)).toBe("About 30 seconds remaining");
  });
});

describe("formatStalled", () => {
  it("wraps the duration in the stalled template", () => {
    expect(formatStalled(t, 125000)).toBe("Stalled for 3 minutes");
  });
});

describe("formatPhaseLabel", () => {
  it("resolves a known phase", () => {
    expect(formatPhaseLabel(t, "opening_database")).toBe("Opening database");
  });
});

describe("formatStepLabel", () => {
  it("strips the startup. prefix before the namespace lookup", () => {
    expect(formatStepLabel(t, step({ label_key: "startup.step.database_backup" }))).toBe(
      "Database backup",
    );
  });
});

describe("formatStepProgress", () => {
  it("renders the progress-unavailable statement for an opaque step", () => {
    expect(formatStepProgress(t, step({ measure: "opaque" }))).toBe(
      "Progress cannot be measured for this step.",
    );
  });

  it("pluralizes a counting step's unit off done", () => {
    expect(formatStepProgress(t, step({ measure: "counting", unit: "bytes", done: 1 }))).toBe(
      "1 byte",
    );
  });

  it("pluralizes a counted step's unit off total", () => {
    expect(
      formatStepProgress(t, step({ measure: "counted", unit: "turns", done: 5, total: 10 })),
    ).toBe("5 of 10 turns");
  });

  it("uses a singular unit when total is 1", () => {
    expect(
      formatStepProgress(t, step({ measure: "counted", unit: "turns", done: 1, total: 1 })),
    ).toBe("1 of 1 turn");
  });
});

describe("percentDone", () => {
  it("computes the plain percentage", () => {
    expect(percentDone(5, 10)).toBe(50);
  });

  it("clamps to 0 when total is 0", () => {
    expect(percentDone(0, 0)).toBe(0);
  });

  it("clamps above 100", () => {
    expect(percentDone(15, 10)).toBe(100);
  });
});
