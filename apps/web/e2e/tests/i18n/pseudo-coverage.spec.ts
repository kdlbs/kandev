import { type Page } from "@playwright/test";

import { test, expect } from "../../fixtures/test-base";

/**
 * Pseudo-locale coverage oracle.
 *
 * Under the pseudo-locale every EXTRACTED message is accented
 * (`Language` → `Ĺàńĝũàĝē`). So any user-facing copy that is still plain ASCII
 * is, by definition, a string that was never wrapped in a Lingui macro. This
 * spec crawls chrome-heavy screens and reports those leftovers.
 *
 * It runs TWO passes, because copy reaches the user by two routes:
 *   - visible text nodes, and
 *   - `COPY_ATTRIBUTES` — the copy a screen-reader user receives and a sighted
 *     one never sees. That pass is why the oracle is no longer blind to an
 *     `aria-label` that was never externalized; see docs/i18n.md.
 *
 * `SCREENS` mirrors `i18nGuardFiles` in eslint.i18n.options.mjs: a screen belongs
 * here once the components that render it have been migrated. The eslint guard
 * only sees plain literals in JSX, so it and this spec cover different halves of
 * the same question — add to both in the PR that migrates a directory.
 *
 * Gated on `KANDEV_I18N_COVERAGE=1` rather than run in CI: with the migration
 * proceeding one directory at a time, a screen's own copy can be clean while
 * shared chrome it renders is not yet. It becomes a hard gate when the last
 * directory lands and the env guard comes off.
 *
 *   KANDEV_I18N_COVERAGE=1 pnpm e2e -- e2e/tests/i18n/pseudo-coverage.spec.ts
 */

const COVERAGE_ENABLED = process.env.KANDEV_I18N_COVERAGE === "1";

/**
 * Migrated screens whose visible text is overwhelmingly UI chrome, not user data.
 *
 * `allow` extends `ALLOWED` for one screen only. Use it for text the frontend
 * must NOT translate but cannot avoid rendering — records the backend owns, or a
 * product name — and say where the value comes from, so the exemption stays
 * auditable instead of quietly widening into a place to hide missed strings.
 */
const SCREENS: Array<{ name: string; url: string; allow?: string[] }> = [
  { name: "settings — appearance", url: "/settings/general/appearance" },
  {
    name: "settings — notifications",
    url: "/settings/general/notifications",
    // Provider names are rows in the notification_providers table. The backend
    // seeds these two (apps/backend/internal/notifications/service/service.go)
    // and users name their own Apprise ones, so they are data on the same
    // footing as a task title. `Apprise` labels the provider type.
    allow: ["Desktop Notifications", "System Notifications", "Apprise"],
  },
  { name: "settings — secrets", url: "/settings/general/secrets" },
  { name: "settings — terminal", url: "/settings/general/terminal" },
  {
    name: "settings — sprites",
    url: "/settings/general/sprites",
    // Product name of the sandbox provider.
    allow: ["Sprites.dev"],
  },
  {
    name: "settings — layouts",
    url: "/settings/general/layouts",
    // NOTE: everything here must be a PERSISTED name. This list is load-bearing
    // in the wrong direction — a broad token also hides genuinely un-migrated
    // copy that happens to match it. "Default" already masked the
    // default-action button once, which rendered a raw English "Default" until
    // `getDefaultActionState` was fixed to resolve it through the catalog.
    //
    // Both groups are display strings that are also PERSISTED, so translating
    // them in place would write locale-dependent values into a user's saved
    // layouts and leave them there after a locale switch:
    //   - Built-in profile names/descriptions (lib/layout/layout-profiles.ts).
    //     `upsertBuiltInLayoutOverride` copies `builtIn.name` into the saved
    //     record the first time a built-in is customized.
    //   - Dockview panel titles (lib/state/layout-manager/constants.ts), which
    //     `toSerializedDockview` writes into the stored layout JSON. That path
    //     is already in `EXCLUDED` in scripts/externalize-strings.mjs.
    // Localizing either needs a key/persisted-value split in those modules.
    allow: [
      "Default",
      "Plan Mode",
      "Preview Mode",
      "VS Code",
      "Agent with Files, Changes, PR Details, and Terminal",
      "Agent and Plan side by side",
      "Agent and Browser side by side",
      "Agent and VS Code side by side",
      "Agent",
      "Plan",
      "Changes",
      "Files",
      "Browser",
      "Terminal",
      "PR Details",
      "Merge Request",
    ],
  },
  {
    name: "settings — keyboard shortcuts",
    url: "/settings/general/keyboard-shortcuts",
    // Modifier/key names label a physical key and are out of scope for
    // translation — the same rule the eslint guard's keyboard pattern encodes.
    //
    // Shortcut names come from CONFIGURABLE_SHORTCUTS in
    // lib/keyboard/shortcut-overrides.ts, a registry shared with the
    // un-migrated voice-mode settings page; it migrates with that page.
    allow: [
      "Ctrl",
      "Shift",
      "Alt",
      "Cmd",
      "Meta",
      "Space",
      "Enter",
      "Tab",
      "Command Panel",
      "Command Panel (Alt)",
      "File Search",
      "Search Task Contents",
      "Quick Chat",
      "Toggle Bottom Terminal",
      "Toggle Sidebar",
      "New Task",
      "Focus Chat Input",
      "Focus CLI Chat Input",
      "Toggle Plan Mode",
      "Recent Task Switcher",
      "Recent Task Switcher (Backward)",
      "Voice Input",
      "Reverse Chat Search",
      "Open Task Pull Request",
    ],
  },
  { name: "settings — task actions", url: "/settings/general/task-actions" },
  // NOT YET: "settings — integrations github", "… gitlab", "… jira",
  // "… linear", "… sentry". Each page's
  // own copy is fully migrated (verified by running this oracle against it —
  // every string the integration owns renders accented), but the route expands
  // the Workspaces > Integrations branch of the settings nav, and
  // `workspaces-group.tsx` / `settings-tree.tsx` are not migrated: "Workspaces",
  // "Integrations", "Automations", "Executors", "Voice Mode", "Utility Agents",
  // "External MCP", "Plugins", "System" and "Toggle theme" all still render plain
  // English there. The /settings/general/* screens above pass because their
  // expanded branch is `general-group.tsx`, which is migrated. Add these entries
  // in the PR that migrates the settings nav — allowlisting those nav labels here
  // would hide real misses instead.
  //
  // Two shared components rendered by all five pages are also still English and
  // would have to be migrated (or allowlisted, which is worse) first:
  // `components/integrations/drafted-integration-enabled-control.tsx`
  // ("Enabled"/"Disabled") and `components/watcher-repository-fields.tsx`
  // ("Repository", "Base Branch", "(no repository)"), plus `STEP_DEFAULT_LABEL`
  // and `stepPlaceholder`. All four are shared with the un-migrated Azure DevOps
  // surface, plus `components/integrations/settings-section.tsx` chrome and the
  // watcher card's own empty/loading states.
  //
  // The Sentry run also surfaced `components/integrations/auth-status-banner.tsx`
  // ("Authenticated", "· checked <relative>") and `@kandev/ui`'s built-in dialog
  // "Close" label — both shared, both out of scope for a per-integration PR.
  //
  // NOT YET, for the same reason: "settings — workspace workflows"
  // (`/settings/workspace/:id/workflows`). The workflow editor's own copy is
  // fully migrated and was verified with this oracle against a live instance,
  // but the route expands the same un-migrated Workspaces branch of the settings
  // nav. It also renders the workspace's own name and its workflow/step names,
  // which are user data — so the entry needs the nav migrated first, not an
  // allowlist.
];

/**
 * Text that is legitimately un-accented under pseudo: brand/proper nouns, code
 * identifiers, and units/symbols. Kept in sync with `words.exclude` in
 * apps/web/eslint.i18n.options.mjs.
 */
const ALLOWED = [
  "Kandev",
  "GitHub",
  "GitLab",
  "Jira",
  "Linear",
  "Slack",
  "Sentry",
  "Azure DevOps",
  "ACP",
  "MCP",
  "SSH",
  "URL",
  "ID",
  "English",
  "Pseudo",
  "QA",
];

async function activatePseudo(page: Page, url: string) {
  await page.goto(url);
  await page.evaluate(() => {
    document.cookie = "kandev_locale=pseudo; path=/; max-age=31536000; SameSite=Lax";
  });
  await page.reload();
  await expect(page.locator("html")).toHaveAttribute("lang", "pseudo", { timeout: 15_000 });
}

/**
 * Attributes that carry display copy rather than a value the app compares.
 * Kept in sync with `jsx-attributes.include` in apps/web/eslint.i18n.options.mjs,
 * so the guard and this oracle answer the same question about the same set: on an
 * allowlisted path lint flags the literal, and here it is checked at render.
 *
 * `aria-labelledby` / `aria-describedby` / `aria-controls` are deliberately
 * absent — they carry element ids, not prose, and are excluded by the guard too.
 *
 * `title` is checked on EVERY element, not just interactive ones. A `title` is
 * exposed as an accessible name or description whatever the element is, and the
 * browser renders its native tooltip on hover for any of them — so "the app never
 * shows this one" is not true of a rendered `title`. Restricting it to
 * interactive elements would need a tag/role/tabindex heuristic, and that
 * heuristic would immediately become the place attribute copy hides. The guard
 * checks `title` unconditionally too; the two stay in step.
 */
const COPY_ATTRIBUTES = ["aria-label", "aria-description", "title", "placeholder", "alt"];

/**
 * Collect copy that looks un-externalized: plain-English visible text nodes, and
 * plain-English values of the five copy-bearing attributes.
 *
 * Attribute findings are returned prefixed with the attribute and the element
 * that carries it (`aria-label on button[data-testid=…]: "More information"`),
 * because an attribute string is invisible on screen — a bare value gives you
 * nothing to grep for and nowhere to look.
 *
 * Two counters make an empty `leftovers` mean something, because a selector that
 * matched no elements reports exactly as clean as a clean screen:
 *   - `inspectedAttributes` — non-empty attribute values examined. Asserted per
 *     screen; this is the one that catches a selector matching nothing.
 *   - `localizedAttributes` — those that rendered FULLY accented, i.e. migrated
 *     attribute copy the pass demonstrably reached. Not asserted per screen: a
 *     page of plain inputs and text can legitimately have none, and secrets,
 *     terminal and sprites do. Pinned once instead, in its own test below.
 */
async function findUnlocalizedCopy(
  page: Page,
  allowed: string[],
): Promise<{ leftovers: string[]; localizedAttributes: string[]; inspectedAttributes: number }> {
  return page.evaluate(
    ({ allowedList, copyAttributes }) => {
      const skipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT", "CODE", "PRE", "SVG"]);
      // A narrower set for attributes. `skipTags` above encodes "this element's
      // CONTENT is not prose", which is a different question from whether its
      // LABEL is: an `aria-label` on an <svg> icon, or a `title` on a <code>
      // chip, is copy the user receives either way. Only the three tags that
      // cannot present a label at all are skipped here.
      const attrSkipTags = new Set(["SCRIPT", "STYLE", "NOSCRIPT"]);
      const wordlike = /[A-Za-z]{4,}/;
      const accented = /[À-ɏ]/;

      // Longest first: stripping "VS Code" before "Agent and VS Code side by
      // side" would leave "side by side" behind and report it as a leftover.
      const tokens = [...allowedList].sort((a, b) => b.length - a.length);

      /** Still word-like ASCII once allowlisted tokens are removed. */
      const hasUnmigratedAscii = (text: string) => {
        if (!text || !wordlike.test(text)) return false;
        let residue = text;
        for (const token of tokens) residue = residue.split(token).join(" ");
        return wordlike.test(residue);
      };

      /** The text pass's rule, unchanged: any accented character clears a node. */
      const looksUnlocalized = (text: string) => !accented.test(text) && hasUnmigratedAscii(text);

      const collectText = () => {
        const found = new Set<string>();
        const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
        let node: Node | null;
        while ((node = walker.nextNode())) {
          const text = (node.textContent ?? "").trim();
          if (!looksUnlocalized(text)) continue;

          const el = node.parentElement;
          if (!el || skipTags.has(el.tagName)) continue;
          // Ignore hidden nodes — not user-visible copy.
          const rect = el.getBoundingClientRect();
          if (rect.width === 0 && rect.height === 0) continue;

          found.add(text.slice(0, 120));
        }
        return [...found];
      };

      /** Enough to find the element in the DOM and in the source. */
      const describe = (el: Element) => {
        const tag = el.tagName.toLowerCase();
        const testId = el.getAttribute("data-testid");
        if (testId) return `${tag}[data-testid=${testId}]`;
        if (el.id) return `${tag}#${el.id}`;
        const role = el.getAttribute("role");
        return role ? `${tag}[role=${role}]` : tag;
      };

      // The text pass drops zero-size nodes because nothing is on screen to read.
      // That test is WRONG for an attribute: a visually hidden control with an
      // `aria-label` is precisely the case this pass exists to catch, and every
      // `sr-only` node measures as good as zero. What actually disqualifies a
      // label is the element not being rendered at all — `display: none`,
      // `visibility: hidden`, or a skipped `content-visibility` subtree — because
      // then no user receives it, sighted or not. `checkVisibility` asks that
      // directly; deliberately WITHOUT `opacityProperty`, since an opacity-0
      // element is still in the accessibility tree.
      const isRendered = (el: Element) =>
        typeof el.checkVisibility !== "function" ||
        el.checkVisibility({ contentVisibilityAuto: true, visibilityProperty: true });

      const seen = new Map<string, { label: string; count: number }>();
      // The positive control. An attribute that HAS been migrated renders as
      // `Mōŕē ĩńfōŕmàţĩōń`, which contains no 4-letter ASCII run — so it is
      // dropped at the `wordlike` gate and never even reaches the `accented`
      // test. Soundness is unaffected (a real miss is pure ASCII and is still
      // caught), but it means a working pass and a pass whose selector matched
      // NOTHING produce identical empty output. Collecting the accented values
      // proves the pass actually reached migrated attribute copy on this
      // screen, so the caller can tell those two apart.
      const localized: string[] = [];
      let inspected = 0;

      const inspectAttribute = (el: Element, attr: string) => {
        // An empty value is never a missed string, and `alt=""` is load-bearing:
        // it marks an image as decorative so screen readers skip it. Reporting
        // one would push authors into writing alt text for spacers.
        const value = (el.getAttribute(attr) ?? "").trim();
        if (!value) return;
        inspected += 1;

        // NOTE the divergence from the text pass, which clears a node the moment
        // it contains one accented character. That rule hides a real shape here:
        // an `aria-label` built as "Collapse ${label}" renders
        // "Collapse Ĝēńēŕàĺ" — an un-migrated English FRAME around a migrated
        // value, which accent-clears-all would call done. Allowlist stripping is
        // what separates it from the legitimate case, where the ASCII fragment is
        // the allowlisted DATA rather than the frame ("Àćţĩōńś ƒōŕ Agent" on the
        // layouts screen strips to nothing).
        if (!hasUnmigratedAscii(value)) {
          // Fully accented, nothing ASCII left: a migrated attribute, and the
          // positive control.
          if (accented.test(value)) {
            localized.push(`${attr} on ${describe(el)}: "${value.slice(0, 60)}"`);
          }
          return;
        }

        // Keyed by attribute+value, not by element: one un-migrated shared
        // component renders on twenty rows, and twenty identical findings bury
        // the other nineteen strings. The first element carrying it is the
        // sample you go and look at.
        const key = `${attr} ${value}`;
        const hit = seen.get(key);
        if (hit) {
          hit.count += 1;
          return;
        }
        const label = `${attr} on ${describe(el)}: "${value.slice(0, 120)}"`;
        seen.set(key, {
          label: accented.test(value) ? `${label} - English frame, migrated value` : label,
          count: 1,
        });
      };

      const collectAttributes = () => {
        const selector = copyAttributes.map((attr) => `[${attr}]`).join(",");
        for (const el of Array.from(document.querySelectorAll(selector))) {
          if (attrSkipTags.has(el.tagName.toUpperCase()) || !isRendered(el)) continue;
          for (const attr of copyAttributes) inspectAttribute(el, attr);
        }
        const leftovers = [...seen.values()].map(({ label, count }) =>
          count > 1 ? `${label} (×${count})` : label,
        );
        return { leftovers, localized, inspected };
      };

      const attributes = collectAttributes();
      return {
        leftovers: [...collectText(), ...attributes.leftovers],
        localizedAttributes: attributes.localized,
        inspectedAttributes: attributes.inspected,
      };
    },
    { allowedList: allowed, copyAttributes: COPY_ATTRIBUTES },
  );
}

test.describe("i18n pseudo-locale coverage", () => {
  test.skip(
    !COVERAGE_ENABLED,
    "Set KANDEV_I18N_COVERAGE=1 to run the string-externalization oracle (hard gate at task-40).",
  );

  for (const screen of SCREENS) {
    test(`no un-externalized copy on ${screen.name}`, async ({ testPage }) => {
      await activatePseudo(testPage, screen.url);
      // Let lazy panels settle before scanning.
      await testPage.waitForTimeout(1_000);

      const { leftovers, localizedAttributes, inspectedAttributes } = await findUnlocalizedCopy(
        testPage,
        [...ALLOWED, ...(screen.allow ?? [])],
      );

      // Control FIRST. A migrated attribute renders accented and so is invisible
      // to the leftover check by construction, which means a screen that is
      // clean and a selector that matched NOTHING report identically. Assert the
      // pass actually read attributes before believing what it did not find.
      expect(
        inspectedAttributes,
        `The attribute pass examined no attribute at all on ${screen.name}, so a ` +
          `green leftover check proves nothing about attribute copy here.`,
      ).toBeGreaterThan(0);

      expect(
        leftovers,
        `Un-externalized strings on ${screen.name}:\n${leftovers.map((s) => `  - ${s}`).join("\n")}` +
          `\n\nAttribute pass examined ${inspectedAttributes} attribute(s), of which ` +
          `${localizedAttributes.length} rendered fully accented.`,
      ).toEqual([]);
    });
  }

  /**
   * The positive control, pinned once rather than per screen.
   *
   * `inspectedAttributes` above proves the selector matched; it does not prove
   * the pass can tell migrated attribute copy apart from a miss. This does, on
   * the one element we know is migrated: `language-settings.tsx` renders
   * `aria-label={t("settings:displayLanguage")}` on `#language-select`, so under
   * pseudo it must come back accented. If this test fails while the screens
   * above pass, the accent detection is broken and every green screen is
   * meaningless — which is the failure this whole file exists to not have.
   *
   * A per-screen version of this assertion was tried and dropped: secrets,
   * terminal and sprites legitimately render no fully-accented attribute at all,
   * so it fired on three screens that were fine.
   */
  test("the attribute pass recognizes migrated attribute copy", async ({ testPage }) => {
    await activatePseudo(testPage, "/settings/general/appearance");
    await testPage.waitForTimeout(1_000);

    const { localizedAttributes } = await findUnlocalizedCopy(testPage, ALLOWED);

    expect(
      localizedAttributes.join("\n"),
      `Expected the migrated aria-label on #language-select to render accented. Got:\n` +
        localizedAttributes.join("\n"),
    ).toContain("language-select");
  });
});
