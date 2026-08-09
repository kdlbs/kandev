import { afterEach, describe, expect, it } from "vitest";
import i18n from "i18next";

import { canonicalPanelTitle, panel, panelTitle, PANEL_REGISTRY } from "./constants";
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

describe("layout persistence", () => {
  it("stores canonical English even when the UI is in another locale", async () => {
    await i18n.changeLanguage("pseudo");

    const serialized = toSerializedDockview(
      layoutWith(["chat", "changes", "browser"]),
      1000,
      800,
      new Map(),
    );
    const stored = (serialized as unknown as { panels: Record<string, { title: string }> }).panels;

    // The tab the user is looking at right now is pseudo-accented...
    expect(panelTitle("browser")).not.toBe("Browser");
    // ...but what lands in the saved layout is not.
    expect(stored["chat"].title).toBe("Agent");
    expect(stored["changes"].title).toBe("Changes");
    expect(stored["browser"].title).toBe("Browser");
  });

  it("keeps the canonical title for every registry panel", () => {
    for (const id of Object.keys(PANEL_REGISTRY)) {
      expect(canonicalPanelTitle(id)).toBe(PANEL_REGISTRY[id].title);
    }
  });
});
