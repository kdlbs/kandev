import { describe, expect, it } from "vitest";
import { t } from "@/lib/i18n";

/**
 * Byte-for-byte English for the two Account route headers, as `SETTINGS_ROUTES`
 * in `src/settings-routes.tsx` rendered them before this migration. They were
 * the last two inline English entries in that table.
 *
 * Same reasoning as `components/settings/system/system-route-copy.test.ts`:
 * every gate in the repo verifies that a key *resolves*, not that it resolves
 * to the sentence the route used to render, so a plausible-looking key reuse
 * can rewrite shipping copy with nothing failing. This table is the check.
 */
const ROUTE_COPY: Array<{ route: string; titleKey: string; title: string; description: string }> = [
  {
    route: "security",
    titleKey: "account:securityPageTitle",
    title: "Profile & password",
    description: "Change your password and review devices signed in to your account.",
  },
  {
    route: "tokens",
    titleKey: "account:tokensPageTitle",
    title: "API tokens",
    description: "Personal access tokens for scripts and CLIs acting as you.",
  },
];

describe("account route copy", () => {
  it.each(ROUTE_COPY)("renders /settings/account/$route unchanged", (entry) => {
    expect(t(entry.titleKey)).toBe(entry.title);
    expect(t(`account:${entry.route}PageDescription`)).toBe(entry.description);
  });
});
