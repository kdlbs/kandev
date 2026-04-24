import { describe, it, expect } from "vitest";
import {
  DEFAULT_ISSUE_PRESETS,
  DEFAULT_PR_PRESETS,
  iconForPresetKey,
  interpolatePromptTemplate,
  resolveIssuePresets,
  resolvePRPresets,
  toTaskPreset,
} from "./action-presets";
import { IconEye, IconSparkles } from "@tabler/icons-react";

describe("interpolatePromptTemplate", () => {
  it("replaces {url} and {title} placeholders", () => {
    const out = interpolatePromptTemplate("Review {url} titled {title}", {
      url: "https://gh/pr/1",
      title: "Fix bug",
    });
    expect(out).toBe("Review https://gh/pr/1 titled Fix bug");
  });

  it("leaves unknown placeholders intact", () => {
    const out = interpolatePromptTemplate("See {url} and {unknown}", {
      url: "u",
      title: "t",
    });
    expect(out).toBe("See u and {unknown}");
  });

  it("handles a template with no placeholders", () => {
    expect(interpolatePromptTemplate("nothing to sub", { url: "u", title: "t" })).toBe(
      "nothing to sub",
    );
  });
});

describe("iconForPresetKey", () => {
  it("returns the mapped Tabler icon for a known key", () => {
    expect(iconForPresetKey("eye")).toBe(IconEye);
  });

  it("falls back to sparkle for unknown or missing keys", () => {
    expect(iconForPresetKey(undefined)).toBe(IconSparkles);
    expect(iconForPresetKey("nonsense")).toBe(IconSparkles);
  });
});

describe("resolvePRPresets / resolveIssuePresets", () => {
  it("uses defaults when stored is null", () => {
    const pr = resolvePRPresets(null);
    expect(pr.length).toBe(DEFAULT_PR_PRESETS.length);
    const issue = resolveIssuePresets(null);
    expect(issue.length).toBe(DEFAULT_ISSUE_PRESETS.length);
  });

  it("uses defaults when the stored list is empty (treats as unset)", () => {
    const pr = resolvePRPresets({ workspace_id: "w", pr: [], issue: [] });
    expect(pr.length).toBe(DEFAULT_PR_PRESETS.length);
  });

  it("uses stored presets when non-empty", () => {
    const stored = {
      workspace_id: "w",
      pr: [
        {
          id: "custom",
          label: "Custom",
          hint: "h",
          icon: "eye",
          prompt_template: "Do it on {url}",
        },
      ],
      issue: [],
    };
    const pr = resolvePRPresets(stored);
    expect(pr).toHaveLength(1);
    expect(pr[0].label).toBe("Custom");
    expect(pr[0].prompt({ url: "https://x", title: "t" })).toBe("Do it on https://x");
  });
});

describe("toTaskPreset", () => {
  it("wires the prompt function to interpolate the stored template", () => {
    const stored = {
      id: "id",
      label: "L",
      hint: "H",
      icon: "message",
      prompt_template: "On {url} for {title}",
    };
    const preset = toTaskPreset(stored);
    expect(preset.prompt({ url: "U", title: "T" })).toBe("On U for T");
  });
});
