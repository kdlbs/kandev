import { afterEach, describe, expect, it } from "vitest";
import i18n from "i18next";

import { canonicalPanelTitle, panel, PANEL_REGISTRY } from "./constants";
import { panelTitle } from "./panel-title";
import { toSerializedDockview } from "./serializer";
import type { LayoutState } from "./types";

/**
 * Panel titles are BOTH display copy and persisted data: `toSerializedDockview`
 * writes them into the stored layout JSON. That is why the registry keeps the
 * two apart — `title` is canonical English for storage, `titleKey` resolves at
 * render for the tab bar.
 *
 * Before the split, two titles were translated at creation and the rest were
 * not, so a saved layout captured whichever locale created it and kept showing
 * it after a switch.
 */
afterEach(async () => {
  await i18n.changeLanguage("en");
});

function layoutWith(panelIds: string[]): LayoutState {
  return {
    columns: [{ id: "col", groups: [{ id: "group", panels: panelIds.map(panel) }] }],
  } as unknown as LayoutState;
}

describe("panelTitle", () => {
  it("resolves a registry panel through the catalog", async () => {
    expect(panelTitle("changes")).toBe("Changes");

    await i18n.changeLanguage("pseudo");
    const pseudo = panelTitle("changes");

    expect(pseudo).not.toBe("Changes");
    expect(pseudo).toMatch(/[^\x20-\x7E]/);
  });

  it("falls back to the given title for a panel the registry does not know", () => {
    expect(panelTitle("diff:src/app.ts", "Diff [app.ts]")).toBe("Diff [app.ts]");
  });

  it("leaves a product name untranslated", async () => {
    await i18n.changeLanguage("pseudo");
    expect(panelTitle("vscode")).toBe("VS Code");
  });

  it("gives every registry panel a title that is not the raw key", () => {
    const leaked = Object.keys(PANEL_REGISTRY).filter((id) => panelTitle(id).includes(":"));
    expect(leaked).toEqual([]);
  });
});

describe("layout round trip", () => {
  const titlesOf = (state: LayoutState) => {
    const serialized = toSerializedDockview(state, 1000, 800, new Map());
    return (serialized as unknown as { panels: Record<string, { title: string }> }).panels;
  };

  it("localizes on the way into dockview", async () => {
    // `toSerializedDockview` feeds `api.fromJSON`, so this is what the tab bar
    // and the layout editor's preview render — it must follow the locale. The
    // first version of this split had the two directions the wrong way round,
    // which the pseudo-coverage oracle caught as English tab titles on the
    // layouts screen while every unit test passed.
    await i18n.changeLanguage("pseudo");
    const stored = titlesOf(layoutWith(["chat", "changes", "browser"]));

    for (const title of ["chat", "changes", "browser"].map((id) => stored[id].title)) {
      expect(title).toMatch(/[^\x20-\x7E]/);
    }
  });

  it("keeps a stored LayoutState in canonical English", () => {
    // `panel()` builds the shape that gets persisted, so it must NOT localize —
    // otherwise the locale that created a layout is frozen into it.
    const built = layoutWith(["chat", "changes", "browser"]).columns[0].groups[0].panels;
    expect(built.map((p) => p.title)).toEqual(["Agent", "Changes", "Browser"]);
  });

  it("builds the same canonical LayoutState in any locale", async () => {
    const english = layoutWith(["chat", "changes", "browser"]);
    await i18n.changeLanguage("pseudo");
    const pseudo = layoutWith(["chat", "changes", "browser"]);

    expect(pseudo).toEqual(english);
  });

  it("keeps the canonical title for every registry panel", () => {
    for (const id of Object.keys(PANEL_REGISTRY)) {
      expect(canonicalPanelTitle(id)).toBe(PANEL_REGISTRY[id].title);
    }
  });
});
