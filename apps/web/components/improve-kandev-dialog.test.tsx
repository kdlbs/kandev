import { describe, expect, it } from "vitest";
import { render } from "@testing-library/react";
import { Trans } from "react-i18next";

/**
 * These sentences interleave markup with a value, so the codemod half-converted
 * them: it keyed the `<code>` contents (`gh`, `kdlbs/kandev`) and left the prose
 * around them as a JSX literal. That renders correctly in English and passes
 * lint, `i18n:check`, `i18n:ratchet` and `i18n:removed` — the word order is just
 * frozen in JSX where no translator can reach it.
 *
 * Rebuilt as one `<Trans>` each. Two things are easy to get wrong and invisible
 * afterwards, so they are pinned here:
 *
 *   1. The `<n>` index must land on the element's position among ALL children,
 *      counting text and `{" "}` expression containers. Prettier reflowing a
 *      line inserts a `{" "}` and silently shifts every index after it.
 *   2. `gh` and `kdlbs/kandev` travel through `values`, not as tag content, so
 *      a translator cannot reword a binary name or a repo slug. They cannot
 *      instead be left as bare children under an empty `<1></1>` tag — that
 *      form drops plain text children (it only preserves element children),
 *      which silently renders "needs the  CLI".
 */
describe("improve-kandev dialog <Trans> copy", () => {
  it("renders the gh-auth notice byte-identically to the old literal", () => {
    const { container } = render(
      <Trans
        i18nKey="common:theFinalStepOpensAPullRequest"
        values={{ binary: "gh", message: "Run gh auth login." }}
      >
        The final step of this workflow opens a pull request, which needs the <code>gh</code> CLI to
        be authenticated. {"Run gh auth login."}
      </Trans>,
    );

    expect(container.textContent).toBe(
      "The final step of this workflow opens a pull request, which needs the gh CLI to be " +
        "authenticated. Run gh auth login.",
    );
    expect(container.querySelector("code")?.textContent).toBe("gh");
  });

  it("renders the fork bullet with the repo slug intact", () => {
    const { container } = render(
      <Trans i18nKey="common:theAgentForksKandevToYourAccount" values={{ repo: "kdlbs/kandev" }}>
        The agent forks <code>kdlbs/kandev</code> to your GitHub account and opens a PR from your
        fork, credited to you
      </Trans>,
    );

    expect(container.textContent).toBe(
      "The agent forks kdlbs/kandev to your GitHub account and opens a PR from your fork, " +
        "credited to you",
    );
    expect(container.querySelector("code")?.textContent).toBe("kdlbs/kandev");
  });

  it("keeps the contributor line one sentence per branch", () => {
    const { container } = render(
      <Trans
        i18nKey="common:contributingAsLogin"
        values={{ login: "octocat", access: "You have write access." }}
      >
        Contributing as <code>@octocat</code>. {"You have write access."}
      </Trans>,
    );

    expect(container.textContent).toBe("Contributing as @octocat. You have write access.");
  });
});
