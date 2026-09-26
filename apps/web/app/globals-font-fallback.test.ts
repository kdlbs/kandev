import { readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, it } from "vitest";

const LOCAL_GLYPH_FAMILY = "NerdFontLocalGlyphs";
const BUNDLED_GLYPH_FAMILY = "NerdFontBundledGlyphs";

const css = readFileSync(join(__dirname, "globals.css"), "utf8");
const themeCss = readFileSync(join(__dirname, "../../packages/theme/src/fonts.css"), "utf8");

// Slice to the rule's closing brace rather than a fixed character budget: a
// fixed window silently truncates as the rule grows, which already caused a
// descriptor to read as absent when it was present.
const fontFace = (family: string) => {
  const faceNameStart = themeCss.indexOf(`font-family: "${family}"`);
  const faceStart = themeCss.lastIndexOf("@font-face", faceNameStart);
  return themeCss.slice(faceStart, themeCss.indexOf("}", faceNameStart) + 1);
};

const localFontFace = fontFace(LOCAL_GLYPH_FAMILY);
const bundledFontFace = fontFace(BUNDLED_GLYPH_FAMILY);

/** The first real font stack declared for `name`, sliced to its terminating
 *  semicolon.
 *
 *  Two traps here. A fixed character budget silently truncates as a stack
 *  grows, so the slice ends at the semicolon. And the first textual match is
 *  the Tailwind `@theme inline` alias (`--font-sans: var(--font-sans);`),
 *  which contains no families at all, so `var()`-only values are skipped. */
const declaration = (name: string) => {
  let from = 0;
  for (;;) {
    const start = css.indexOf(name, from);
    if (start === -1) throw new Error(`no font stack found for ${name}`);
    const end = css.indexOf(";", start) + 1;
    const text = css.slice(start, end);
    if (!/:\s*var\([^;]*\);$/.test(text.replace(/\s+/g, " "))) return text;
    from = end;
  }
};

describe("Nerd Font PUA glyph fallback", () => {
  it("declares separate local and bundled font faces", () => {
    expect(localFontFace).toContain(`font-family: "${LOCAL_GLYPH_FAMILY}"`);
    expect(bundledFontFace).toContain(`font-family: "${BUNDLED_GLYPH_FAMILY}"`);
  });

  it("lets installed fonts cover all private use ranges", () => {
    expect(localFontFace).toContain("U+E000-F8FF");
    expect(localFontFace).toContain("U+F0000-FFFFD");
    expect(localFontFace).toContain("U+100000-10FFFD");
  });

  it("limits the bundled face to codepoints present in the subset", () => {
    const ranges = [
      "U+E0A0-E0A3",
      "U+E0B0-E0C8",
      "U+E0CA",
      "U+E0CC-E0D2",
      "U+E0D4",
      "U+E5FA-E6B7",
      "U+E700-E8EF",
      "U+F400-F533",
    ];

    for (const range of ranges) expect(bundledFontFace).toContain(range);
    expect(bundledFontFace).not.toContain("U+E000-F8FF");
    expect(bundledFontFace).not.toContain("U+F0000-FFFFD");
    expect(bundledFontFace).not.toContain("U+100000-10FFFD");
  });

  it("keeps local and bundled sources in different faces", () => {
    expect(localFontFace).toContain("local(");
    expect(localFontFace).not.toContain("url(");
    expect(bundledFontFace).toContain("url(");
    expect(bundledFontFace).not.toContain("local(");
  });

  it("serves the bundled subset from the app, not a third-party CDN", () => {
    // A remote origin would leak that a user pasted terminal output, and add a
    // runtime dependency on someone else's uptime.
    const urls = [...bundledFontFace.matchAll(/url\("([^"]+)"\)/g)].map((m) => m[1]);

    expect(urls).toHaveLength(1);
    expect(urls[0]).toBe("/fonts/nerd-symbols/nerd-symbols-subset-bca747e8.woff2");
  });

  it("names every source by full font name, not by family name", () => {
    // `local()` matches a full font name or PostScript name only. A family
    // name silently fails and falls through to the next source, so an entry
    // like local("MesloLGS") looks correct and renders nothing. Verified
    // against Chromium: "MesloLGS" fails, "MesloLGS Nerd Font Regular" resolves.
    const sources = [...localFontFace.matchAll(/local\("([^"]+)"\)/g)].map((m) => m[1]);

    expect(sources.length).toBeGreaterThan(1);
    for (const source of sources) {
      // Nerd Fonts v3.5.0 uses both full names ("Nerd Font") and PostScript
      // aliases ("NF", "NFM", or "NFP"). A bare family name matches neither.
      const isFullName = source.includes("Nerd Font");
      const isPostScriptName = /(?:NF|NFM|NFP)(?:[- ]|$)/.test(source);
      expect(isFullName || isPostScriptName).toBe(true);
    }
  });

  it("uses the upstream names for families whose aliases are not predictable", () => {
    const sources = [...localFontFace.matchAll(/local\("([^"]+)"\)/g)].map((m) => m[1]);
    const required = [
      "CaskaydiaCove NF Regular",
      "CaskaydiaCoveNF-Regular",
      "JetBrainsMono NF Regular",
      "JetBrainsMonoNF-Regular",
      "BigBlueTerm437 Nerd Font",
      "BigBlueTerm437NF",
      "D2KodingLigature Nerd Font",
      "D2KodingLigatureNF",
    ];

    for (const source of required) expect(sources).toContain(source);
  });

  it("lists only Nerd Font patched faces, never an unpatched base font", () => {
    // An unpatched base font adds no intended coverage. It can also claim an
    // unrelated PUA glyph and preempt the bundled face for that character.
    // Unpatched Cascadia Code shipped here once and covers 0 of the powerline,
    // seti, devicon and octicon codepoints.
    const sources = [...localFontFace.matchAll(/local\("([^"]+)"\)/g)].map((m) => m[1]);

    for (const source of sources) {
      const isPatched = source.includes("Nerd Font") || /(?:NF|NFM|NFP)(?:[- ]|$)/.test(source);
      expect(isPatched, `${source} is not a Nerd Font patched face`).toBe(true);
    }
  });

  it("uses swap font-display so text is never invisible while the subset loads", () => {
    // Without it the browser blocks rendering for up to 3s on a slow
    // connection, hiding the surrounding text as well as the glyphs.
    expect(bundledFontFace).toContain("font-display: swap");
  });

  it("scales the glyphs down to sit inline with proportional text", () => {
    // Powerline separators fill a full terminal cell, so unscaled they render
    // at roughly twice the cap height of the UI face. Measured 1.99x before
    // this descriptor, 1.43x after.
    expect(localFontFace).toContain("size-adjust: 75%");
    expect(bundledFontFace).toContain("size-adjust: 75%");
  });

  it("is reachable from every font stack in the stylesheet", () => {
    // Asserting two named stacks was not enough: .markdown-body and
    // .chat-message-list hardcode their own families instead of reading the
    // variables, so rendered chat messages kept showing notdef boxes while the
    // composer rendered glyphs. Scan every stack rather than a chosen few.
    const outsideFontFace = css;
    const stacks = [...outsideFontFace.matchAll(/(?:font-family|--font-[a-z-]+):\s*([^;]+);/g)]
      .map((m) => ({ value: m[1].replace(/\s+/g, " ").trim() }))
      .filter((s) => !s.value.startsWith("var(") && /[,]/.test(s.value));

    expect(stacks.length).toBeGreaterThan(3);
    for (const stack of stacks) {
      expect(stack.value).toContain(LOCAL_GLYPH_FAMILY);
      expect(stack.value).toContain(BUNDLED_GLYPH_FAMILY);
      expect(stack.value.indexOf(LOCAL_GLYPH_FAMILY)).toBeLessThan(
        stack.value.indexOf(BUNDLED_GLYPH_FAMILY),
      );
    }
  });

  it("keeps the UI typeface ahead of the glyph fallback for ordinary text", () => {
    const sans = declaration("--font-sans:");

    expect(sans.indexOf("Figtree")).toBeLessThan(sans.indexOf(LOCAL_GLYPH_FAMILY));
    expect(sans.indexOf(LOCAL_GLYPH_FAMILY)).toBeLessThan(sans.indexOf(BUNDLED_GLYPH_FAMILY));
  });
});
