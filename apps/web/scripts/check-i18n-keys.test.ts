import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { spawnSync } from "node:child_process";

import { describe, expect, it } from "vitest";

type Namespace = Record<string, string | number>;
type Locale = Record<string, Namespace>;
type Fixture = Record<string, Locale>;

const ZH_CN = "zh-cn";

const SCRIPT = path.resolve(import.meta.dirname, "check-i18n-keys.mjs");
const CATALOG_HELPER = path.resolve(import.meta.dirname, "lib", "i18n-catalogs.mjs");
const COPY_HELPER = path.resolve(import.meta.dirname, "lib", "looks-like-copy.mjs");
// The script reads the shared `words.exclude` list from here, so the fixture
// needs it at the same relative position (`../eslint.i18n.options.mjs`).
const GUARD_OPTIONS = path.resolve(import.meta.dirname, "..", "eslint.i18n.options.mjs");

function runFixture(fixture: Fixture, extras: Record<string, unknown> = {}) {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-i18n-keys-"));
  try {
    const scriptsDir = path.join(root, "scripts");
    mkdirSync(scriptsDir, { recursive: true });
    const script = path.join(scriptsDir, "check-i18n-keys.mjs");
    copyFileSync(SCRIPT, script);
    const libDir = path.join(scriptsDir, "lib");
    mkdirSync(libDir, { recursive: true });
    copyFileSync(CATALOG_HELPER, path.join(libDir, "i18n-catalogs.mjs"));
    copyFileSync(COPY_HELPER, path.join(libDir, "looks-like-copy.mjs"));
    copyFileSync(GUARD_OPTIONS, path.join(root, "eslint.i18n.options.mjs"));

    for (const [locale, namespaces] of Object.entries(fixture)) {
      const localeDir = path.join(root, "src", "locales", locale);
      mkdirSync(localeDir, { recursive: true });
      for (const [namespace, messages] of Object.entries(namespaces)) {
        writeFileSync(path.join(localeDir, `${namespace}.json`), JSON.stringify(messages));
      }
    }

    // `"<locale>/_verbatim"` writes src/locales/<locale>/_verbatim.json;
    // `"_verbatim"` writes the shared one beside the locale directories.
    for (const [target, contents] of Object.entries(extras)) {
      const file = path.join(root, "src", "locales", `${target}.json`);
      mkdirSync(path.dirname(file), { recursive: true });
      writeFileSync(file, JSON.stringify(contents));
    }

    return spawnSync(process.execPath, [script], { encoding: "utf8" });
  } finally {
    rmSync(root, { recursive: true, force: true });
  }
}

const completeFixture = (): Fixture => ({
  en: { common: { first: "First", second: "Second" }, settings: { third: "Third" } },
  pseudo: { common: { first: "Ƒîřśţ", second: "Śēćøńđ" }, settings: { third: "Ţĥîřđ" } },
  [ZH_CN]: { common: { first: "第一", second: "第二" }, settings: { third: "第三" } },
});

describe("real locale catalog parity", () => {
  it("accepts a complete real locale", () => {
    const result = runFixture(completeFixture());
    expect(result.status).toBe(0);
  });

  it("accepts an identical translated value for a technical literal", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Kandev";
    fixture[ZH_CN].common.first = "Kandev";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
  });

  it("fails on a missing namespace", () => {
    const fixture = completeFixture();
    delete fixture[ZH_CN].settings;
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("settings");
    expect(result.stderr).toContain("missing namespace");
  });

  it("fails on an extra namespace", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].extra = { surprise: "意外" };
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("extra");
    expect(result.stderr).toContain("extra namespace");
  });

  it("fails on a missing key", () => {
    const fixture = completeFixture();
    delete fixture[ZH_CN].common.second;
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("missing key: second");
  });

  it("fails on an extra key", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.surprise = "意外";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("extra key: surprise");
  });

  it("fails on a translation that drops an interpolation placeholder", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open {{url}} as {{user}}";
    fixture[ZH_CN].common.first = "以 {{user}} 身份打开";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("interpolation placeholder mismatch: first");
  });

  it("fails on a translation that drops numeric Trans tags", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open <1>{{url}}</1>";
    fixture[ZH_CN].common.first = "打开 {{url}}";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("Trans tag structure mismatch: first");
  });

  it("fails on a translation that changes a numeric Trans tag index", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open <1>{{url}}</1>";
    fixture[ZH_CN].common.first = "打开 <2>{{url}}</2>";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("Trans tag structure mismatch: first");
  });
});

describe("translated value validation", () => {
  it("fails on an empty translated value", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = "";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("empty translated value: first");
  });

  it("fails on a whitespace-only translated value", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = " \t\n ";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("empty translated value: first");
  });

  it("fails on a non-string translated value", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = 42;
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("non-string translated value: first");
    expect(result.stderr).not.toContain("matchAll is not a function");
  });
});

/**
 * `en` <-> `pseudo` gates, and gated alone while real locales were advisory, so
 * it has its own coverage rather than being assumed. Both catalogs are ours:
 * English is authored beside the code and `pseudo` is generated from it, so a
 * mismatch is always the author's to fix in the same change.
 */
describe("pseudo catalog drift still fails the build", () => {
  it("rejects a key missing from pseudo", () => {
    const fixture = completeFixture();
    delete fixture.pseudo.common.second;
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("pseudo catalog is out of sync");
  });

  it("rejects a key present only in pseudo", () => {
    const fixture = completeFixture();
    fixture.pseudo.common.surprise = "Śũřƥřîśē";
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("pseudo catalog is out of sync");
  });

  it("still fails on pseudo drift even when a real locale is also broken", () => {
    const fixture = completeFixture();
    delete fixture.pseudo.common.second;
    delete fixture[ZH_CN].common.second;
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("pseudo catalog is out of sync");
  });
});

/**
 * The identical-to-English check, and the two tiers that keep its exemption
 * mechanism from becoming a shrinking allowlist. The structural checks above all
 * compare shapes, so a catalog copy-pasted from `en` used to pass every one of
 * them; these are what closes that.
 */
describe("identical-to-English detection", () => {
  it("rejects a catalog copy-pasted wholesale from en", () => {
    const fixture = completeFixture();
    fixture[ZH_CN] = structuredClone(fixture.en);
    const result = runFixture(fixture);

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("value identical to English");
  });

  it("accepts an identical value the shared copy predicate calls non-copy", () => {
    // Tier 1: no declaration needed. "Kandev" is a brand noun, "{{count}}" is a
    // frame with no prose in it — `looksLikeCopy` rejects both.
    const fixture = completeFixture();
    fixture.en.common.first = "Kandev";
    fixture.pseudo.common.first = "Kandev";
    fixture[ZH_CN].common.first = "Kandev";
    fixture.en.common.second = "{{count}}";
    fixture.pseudo.common.second = "{{count}}";
    fixture[ZH_CN].common.second = "{{count}}";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
  });

  it("accepts a declared verbatim value and names the registry when it is not", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = fixture.en.common.first as string;

    expect(runFixture(fixture).status).toBe(1);

    const declared = runFixture(fixture, {
      [`${ZH_CN}/_verbatim`]: { "common:first": "same word in this language" },
    });
    expect(declared.status).toBe(0);
  });

  it("rejects a verbatim entry with no reason", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = fixture.en.common.first as string;
    const result = runFixture(fixture, { [`${ZH_CN}/_verbatim`]: { "common:first": "" } });

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("verbatim entry with no reason");
  });

  it("rejects a verbatim entry the locale has since translated", () => {
    // Deleting an entry cannot make anything pass, and neither can leaving a
    // dead one behind. That is the difference from an allowlist you can shrink.
    const fixture = completeFixture();
    const result = runFixture(fixture, {
      [`${ZH_CN}/_verbatim`]: { "common:first": "no longer true, zh-cn translates it" },
    });

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("every locale translates this");
  });

  it("rejects a verbatim entry for a key that no longer exists", () => {
    const fixture = completeFixture();
    const result = runFixture(fixture, {
      [`${ZH_CN}/_verbatim`]: { "common:deleted": "points at nothing" },
    });

    expect(result.status).toBe(1);
    expect(result.stderr).toContain("no such key in en");
  });
});

describe("what the real-locale pass still does not decide", () => {
  it("accepts swapped plural forms", () => {
    const fixture = completeFixture();
    fixture.en.common = { items_one: "{{count}} item", items_other: "{{count}} items" };
    fixture.pseudo.common = { items_one: "{{count}} îţēḿ", items_other: "{{count}} îţēḿś" };
    fixture[ZH_CN].common = { items_one: "{{count}} 个项目", items_other: "{{count}} 个项目" };
    const swapped = structuredClone(fixture);
    swapped[ZH_CN].common = {
      items_one: fixture[ZH_CN].common.items_other,
      items_other: fixture[ZH_CN].common.items_one,
    };
    const result = runFixture(swapped);

    expect(result.status).toBe(0);
  });
});
