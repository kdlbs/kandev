import { cleanup, render, screen } from "@testing-library/react";
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { NetworkPoliciesCard } from "./sprites-sections";
import { SpritesApiKeyCard } from "./sprites-api-key-card";

vi.mock("@/hooks/domains/settings/use-sprites", () => ({
  useSprites: () => ({ status: { connected: true, instance_count: 2, token_configured: true } }),
}));
vi.mock("@/lib/api/domains/sprites-api", () => ({ testSpritesConnection: vi.fn() }));

/**
 * Pseudo-locale acceptance for the two Sprites-only cards.
 *
 * These mount only on a profile whose executor is `sprites`, and the e2e mock
 * backend refuses to create one (`POST .../profiles` → 500 "profile not
 * created"), so the pseudo-locale click-through that covered every other card
 * in this directory cannot reach them. Asserting here that their copy renders
 * accented is the same check: any string still plain ASCII was never routed
 * through `t()`.
 */
const ASCII_WORD = /\b[A-Za-z]{4,}\b/;

beforeAll(async () => {
  await activateLocale("pseudo");
});

afterAll(async () => {
  await activateLocale("en");
});

afterEach(cleanup);

function plainEnglishIn(container: HTMLElement): string[] {
  const skip = new Set(["SCRIPT", "STYLE", "CODE", "PRE"]);
  const accented = /[À-ɏ]/;
  const found: string[] = [];
  const walker = document.createTreeWalker(container, NodeFilter.SHOW_TEXT);
  let node = walker.nextNode();
  while (node) {
    const text = (node.textContent ?? "").trim();
    const tag = node.parentElement?.tagName ?? "";
    if (text && !skip.has(tag) && ASCII_WORD.test(text) && !accented.test(text)) {
      found.push(text);
    }
    node = walker.nextNode();
  }
  return found;
}

describe("Sprites cards under the pseudo-locale", () => {
  it("renders every NetworkPoliciesCard string through the catalog", () => {
    const { container } = render(
      <NetworkPoliciesCard
        rules={[{ domain: "example.com", action: "allow" }]}
        baselineRules={[]}
        onRulesChange={() => {}}
      />,
    );

    // `example.com` is a rule the user typed — data, not copy.
    expect(plainEnglishIn(container).filter((s) => s !== "example.com")).toEqual([]);
  });

  it("renders every SpritesApiKeyCard string through the catalog", () => {
    const { container } = render(
      <SpritesApiKeyCard
        secretId="secret-1"
        baselineSecretId="secret-1"
        onSecretIdChange={() => {}}
        secrets={[{ id: "secret-1", name: "SPRITES_TOKEN" } as never]}
      />,
    );

    // The secret's name is user data.
    expect(plainEnglishIn(container).filter((s) => s !== "SPRITES_TOKEN")).toEqual([]);
  });

  it("selects the plural sprite-count form from count, not a concatenated 's'", async () => {
    await activateLocale("en");
    const { container } = render(
      <SpritesApiKeyCard secretId="secret-1" onSecretIdChange={() => {}} secrets={[]} />,
    );

    expect(screen.getByText("Connected. 2 active sprites.")).toBeTruthy();
    expect(container.textContent).not.toContain("sprite(s)");
    await activateLocale("pseudo");
  });
});
