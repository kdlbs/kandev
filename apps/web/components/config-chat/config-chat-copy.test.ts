import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";

/**
 * Byte-for-byte English for every string this migration externalized, as the
 * Configuration Chat surfaces rendered them before it.
 *
 * An i18n migration must not change copy, and nothing else in the toolchain can
 * tell you that it did: `i18n:check` proves a key resolves and that `en` and
 * `pseudo` agree, lint proves nothing is hardcoded, and the pseudo-coverage
 * oracle proves nothing is left in English. All four are green when a key
 * resolves to a *different* sentence than the one it replaced — see
 * docs/i18n.md, "An existing key is not automatically the right key".
 *
 * Two entries here are the ones that hazard actually applies to:
 *
 *   - `common:configurationChat` replaced `common:commandConfigurationChat`,
 *     which held the byte-identical "Configuration Chat" for the command
 *     palette. Folding them avoids a twin that could drift; this pins that the
 *     fold did not move the palette's label.
 *   - `common:cancel` is reused for the setup footer's Cancel button, which
 *     rendered a bare literal "Cancel" before.
 */
const CONFIG_CHAT_COPY: Array<[key: string, english: string]> = [
  // The feature name, rendered by the FAB, the panel header, the dialog
  // heading and the command palette. One key, four surfaces.
  ["common:configurationChat", "Configuration Chat"],
  ["common:cancel", "Cancel"],
  // The panel chrome.
  ["configChat:openInQuickChat", "Open in Quick Chat"],
  ["configChat:closePanel", "Close configuration chat"],
  ["configChat:tagline", "Configure Kandev with natural language"],
  // The setup surface, shared by the floating panel and the quick-chat dialog.
  ["configChat:agentProfileHeading", "Configuration agent profile"],
  [
    "configChat:agentProfileHelp",
    "Choose the agent with access to configuration tools. This becomes the workspace default.",
  ],
  ["configChat:tryAsking", "Try asking"],
  ["configChat:promptHeading", "What would you like to configure?"],
  ["configChat:promptPlaceholder", "Ask anything about your configuration..."],
  ["configChat:startChat", "Start chat"],
  ["configChat:startingChat", "Starting chat..."],
  ["configChat:startChatAria", "Start configuration chat"],
  [
    "configChat:dialogDescription",
    "Ask an agent to manage workflows, agent profiles, and MCP configuration.",
  ],
  [
    "configChat:noAgentProfiles",
    "No agent profiles are available. Create one in Agent settings first.",
  ],
  // Owned by `use-config-chat.ts`, which holds no JSX — lint reported the file
  // as clean while both of these reached the user.
  [
    "configChat:profileUnavailable",
    "The selected agent profile is not available yet. Try again shortly.",
  ],
  ["configChat:unknownError", "Unknown error"],
];

/**
 * `SUGGESTION_PROMPTS` was a SCREAMING_CASE array, which
 * `i18next/no-literal-string` skips entirely — four sentences the guard never
 * reported. They are display copy the user then SENDS as a prompt, so they are
 * translated; pinning them keeps that intentional.
 */
const SUGGESTION_COPY: Array<[key: string, english: string]> = [
  ["configChat:suggestionAddReviewStep", "Add a 'Code Review' step to my workflow"],
  ["configChat:suggestionCreateProfile", "Create a new agent profile with auto-approve enabled"],
  ["configChat:suggestionShowWorkflow", "Show me the current workflow configuration"],
  ["configChat:suggestionUpdateMcp", "Update the MCP servers for the default agent profile"],
];

/**
 * `@kandev/ui` primitives must not import the app's i18n runtime, so their
 * English defaults are overridden at the call site. These two are the labels
 * the pseudo-coverage oracle reported on every migrated screen.
 */
const SHARED_CHROME_COPY: Array<[key: string, english: string]> = [
  // `Breadcrumb`'s own `aria-label="breadcrumb"` default, lowercase as shadcn
  // ships it. Overridden by page-topbar, task-top-bar-title and OfficeSimplePane.
  ["common:breadcrumb", "breadcrumb"],
  // sonner's `containerAriaLabel`, which defaults to "Notifications". Kept
  // separate from `settings:notifications` (the settings PAGE name, per
  // SEGMENT_LABEL_KEYS) on purpose: renaming that page must not rename the
  // toast region a screen reader announces.
  ["common:toastRegionLabel", "Notifications"],
];

describe("Configuration Chat English copy", () => {
  it.each([...CONFIG_CHAT_COPY, ...SUGGESTION_COPY, ...SHARED_CHROME_COPY])(
    "%s renders its pre-migration English",
    (key, english) => {
      expect(t(key)).toBe(english);
    },
  );

  it("does not leave a twin of the feature name behind", () => {
    // A missing key echoes itself, so this asserts the old palette-only key is
    // really gone rather than quietly resolving to the same sentence.
    expect(t("common:commandConfigurationChat")).toBe("commandConfigurationChat");
  });

  it("gives the toast region the same English as the settings page name, from its own key", () => {
    // Note this asserts EQUALITY, and a failure is not necessarily a bug.
    //
    // The two are separate keys with different owners: `settings:notifications`
    // is the settings PAGE name (per SEGMENT_LABEL_KEYS), while
    // `common:toastRegionLabel` names the global toast region a screen reader
    // announces. They happen to render the same word today.
    //
    // So a red here means only "they have diverged". Decide which one you meant
    // to reword, change that one, and update this expectation — do NOT make the
    // other follow, which is the coupling the two keys exist to prevent.
    expect(t("common:toastRegionLabel")).toBe(t("settings:notifications"));
  });
});
