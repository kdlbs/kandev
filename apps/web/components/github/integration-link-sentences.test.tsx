import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import { Trans } from "react-i18next";

import Link from "@/components/routing/app-link";

afterEach(cleanup);

/**
 * The "not connected" notices on the GitHub and GitLab task pages are one
 * sentence wrapped around a settings link. Migrating them created a failure that
 * no gate catches: written as
 *
 *   <Trans i18nKey="…"><Link href={…} /> to see your pull requests…</Trans>
 *
 * with a message of `<0></0> to see your pull requests…`, the tag is EMPTY and
 * the `<Link />` is self-closing, so the link renders with **no text at all** —
 * a visibly broken control on a page whose only purpose is to send you to
 * settings. `pnpm lint`, `i18n:check` and `check-trans-indices.mjs` were all
 * green on it: the key resolved, the catalogs matched, the `<0>` landed on a
 * real element child. Only looking at it showed the gap.
 *
 * The fix is that the link LABEL belongs inside the message, so a translator can
 * move it and it can never render empty. These assert the reassembled sentence
 * and that the anchor carries text.
 */
function renderNotice(i18nKey: string, href: string) {
  return render(
    <Trans i18nKey={i18nKey}>
      <Link href={href} className="underline font-medium cursor-pointer" />
    </Trans>,
  );
}

describe("integration not-connected notices", () => {
  it("renders the GitHub sentence with a non-empty settings link", () => {
    const { container } = renderNotice(
      "github:openSettingsToSeePrsAndIssues",
      "/settings/integrations/github",
    );

    expect(container.textContent).toBe(
      "Open GitHub settings to see your pull requests and issues.",
    );
    const link = screen.getByRole("link");
    expect(link.textContent).toBe("Open GitHub settings");
    expect(link.getAttribute("href")).toBe("/settings/integrations/github");
  });

  it("renders the GitLab sentence with a non-empty settings link", () => {
    const { container } = renderNotice(
      "gitlab:openSettingsToSeeMrsAndIssues",
      "/settings/integrations/gitlab",
    );

    expect(container.textContent).toBe("Settings → GitLab to see your merge requests and issues.");
    expect(screen.getByRole("link").textContent).toBe("Settings → GitLab");
  });
});
