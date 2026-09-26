import { describe, expect, it } from "vitest";
import { detailTabHref, parseDetailTab, RUNS_FEED_HREF } from "./runs-view";

const AUTO_1 = "auto-1";

describe("parseDetailTab", () => {
  it("lands on Activity when no tab is named", () => {
    expect(parseDetailTab(undefined)).toBe("activity");
  });

  it("opens Configure only when it is asked for by name", () => {
    expect(parseDetailTab("configure")).toBe("configure");
  });

  it("falls back to Activity for anything it does not recognise", () => {
    // This surface exists because reading was buried under editing, so a stale
    // bookmark or a typo must never land the user in the editor.
    expect(parseDetailTab("settings")).toBe("activity");
    expect(parseDetailTab("")).toBe("activity");
    expect(parseDetailTab("CONFIGURE")).toBe("activity");
  });
});

describe("detailTabHref", () => {
  it("identifies the automation in both tabs", () => {
    expect(detailTabHref(AUTO_1, "activity")).toBe("/automations/auto-1");
    expect(detailTabHref(AUTO_1, "configure")).toBe("/automations/auto-1?tab=configure");
  });

  it("round-trips through the parser", () => {
    const href = detailTabHref(AUTO_1, "configure");
    const tab = new URL(href, "https://kandev.local").searchParams.get("tab") ?? undefined;

    expect(parseDetailTab(tab)).toBe("configure");
  });
});

describe("RUNS_FEED_HREF", () => {
  it("keeps the flat feed on a query param so no id-shaped segment is reserved", () => {
    const url = new URL(RUNS_FEED_HREF, "https://kandev.local");

    expect(url.pathname).toBe("/automations");
    expect(url.searchParams.get("view")).toBe("feed");
  });
});
