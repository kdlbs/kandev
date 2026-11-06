import { describe, expect, it } from "vitest";
import {
  CONTROL_VIEWS,
  CONTROL_VIEW_LABEL_KEYS,
  controlHref,
  controlView,
  isIntegratedControlPath,
} from "./integrated-control";

describe("Integrated control destinations (AC-UI-INTEGRATED-CONTROL-001.2)", () => {
  it.each(["/control", "/control/", ...CONTROL_VIEWS.map((view) => `/control/${view}`)])(
    "recognizes the bounded route family %s",
    (path) => expect(isIntegratedControlPath(path)).toBe(true),
  );
  it.each(["/", "/controls", "/controller/activity", "/tasks/control"])(
    "does not capture ordinary routes %s",
    (path) => expect(isIntegratedControlPath(path)).toBe(false),
  );
  it.each(CONTROL_VIEWS)("round trips %s without a synthetic workspace", (view) => {
    expect(controlHref(view)).toBe(`/control/${view}`);
    expect(controlView(controlHref(view))).toBe(view);
  });
  it.each(["/control", "/control/", "/control/unknown", "/control/activity/extra"])(
    "falls back to overview for %s",
    (path) => expect(controlView(path)).toBe("overview"),
  );
  it("encodes original workspace and coordinator identifiers as independent query values", () => {
    const url = new URL(
      controlHref("history", "workspace /?&", "coordinator +#"),
      "https://test.invalid",
    );
    expect(url.pathname).toBe("/control/history");
    expect([...url.searchParams]).toEqual([
      ["workspace", "workspace /?&"],
      ["coordinator", "coordinator +#"],
    ]);
    expect(controlHref("overview", undefined, "coordinator")).toBe(
      "/control/overview?coordinator=coordinator",
    );
  });
});

it.each([
  ["overview", "sidebar:integratedControl.views.overview"],
  ["activity", "sidebar:integratedControl.views.activity"],
  ["workspaces", "common:workspaces"],
  ["providers", "sidebar:integratedControl.views.providers"],
  ["history", "sidebar:integratedControl.views.history"],
] as const)(
  "shares the canonical %s label between control navigation and its header",
  (view, key) => {
    expect(CONTROL_VIEW_LABEL_KEYS[view]).toBe(key);
  },
);
