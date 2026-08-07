import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";
import sidebarCatalog from "@/src/locales/en/sidebar.json";

/**
 * Byte-for-byte pin on the settings nav's English copy.
 *
 * Externalizing a string can silently REWRITE it: point a label at an existing
 * key whose wording has drifted and lint, `i18n:check`, the ratchet and the unit
 * suite all still pass — the string is externalized, the key exists, the
 * catalogs are in sync. Nothing downstream compares the rendered sentence to
 * what users saw before.
 *
 * Nav labels are the sharpest case, because the same word usually exists twice:
 * once here, and once as the route's own page title in
 * `components/settings/settings-layout-client.tsx`. These assertions are the
 * diff, kept.
 */
const NAV_LABELS: Array<[key: string, english: string]> = [
  ["common:prompts", "Prompts"],
  ["settings:voiceMode", "Voice Mode"],
  ["settings:utilityAgents", "Utility Agents"],
  ["settings:secrets", "Secrets"],
  ["common:externalMcp", "External MCP"],
  ["common:plugins", "Plugins"],
  ["common:workspaces", "Workspaces"],
  ["sidebar:repositories", "Repositories"],
  ["workflows:workflows", "Workflows"],
  ["common:integrations", "Integrations"],
  ["common:automations", "Automations"],
  ["common:executors", "Executors"],
  ["common:agents", "Agents"],
  ["common:settings", "Settings"],
  ["sidebar:account", "Account"],
  ["sidebar:profileAndPassword", "Profile & Password"],
  ["sidebar:apiTokens", "API Tokens"],
];

/**
 * Labels that also exist as a route page title, keyed by
 * `SEGMENT_LABEL_KEYS` in components/settings/settings-layout-client.tsx. The
 * nav and the title are the same words for the same destination and the user
 * sees both at once (nav on the left, title above), so they must resolve
 * through ONE key. Two keys with identical English today is a translator's
 * licence to make them disagree tomorrow.
 *
 * `sidebar:voiceMode` and `sidebar:utilityAgents` were exactly that mistake and
 * were removed in favour of the existing `settings:` keys.
 */
const OWNED_BY_ANOTHER_NAMESPACE = [
  "Prompts",
  "Voice Mode",
  "Utility Agents",
  "Secrets",
  "External MCP",
  "Plugins",
  "Executors",
  "Integrations",
  "Automations",
  "Workspaces",
  "Agents",
  "Settings",
  "Workflows",
];

describe("settings nav copy", () => {
  it.each(NAV_LABELS)("renders %s as its pinned English", (key, english) => {
    expect(t(key)).toBe(english);
  });

  it("keeps every nav label non-empty and free of a leaking key", () => {
    for (const [key] of NAV_LABELS) {
      const value = t(key);
      expect(value).not.toBe("");
      // A missing key echoes the key itself, which reads as copy in the UI.
      expect(value).not.toBe(key);
      expect(value).not.toContain(":");
    }
  });
});

describe("settings nav vs route title", () => {
  it("does not re-declare a label another namespace already owns", () => {
    const duplicates = Object.entries(sidebarCatalog)
      .filter(([, value]) => OWNED_BY_ANOTHER_NAMESPACE.includes(value))
      .map(([key, value]) => `sidebar:${key} = ${JSON.stringify(value)}`);

    expect(
      duplicates,
      "These labels already have a key in common:/settings:/workflows: and are " +
        "rendered as the route's page title from that key. Reuse it instead of " +
        "adding a sidebar: twin, or the nav and the title can drift apart.",
    ).toEqual([]);
  });
});
