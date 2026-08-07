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

function runFixture(fixture: Fixture) {
  const root = mkdtempSync(path.join(tmpdir(), "kandev-i18n-keys-"));
  try {
    const scriptsDir = path.join(root, "scripts");
    mkdirSync(scriptsDir, { recursive: true });
    const script = path.join(scriptsDir, "check-i18n-keys.mjs");
    copyFileSync(SCRIPT, script);
    const libDir = path.join(scriptsDir, "lib");
    mkdirSync(libDir, { recursive: true });
    copyFileSync(CATALOG_HELPER, path.join(libDir, "i18n-catalogs.mjs"));

    for (const [locale, namespaces] of Object.entries(fixture)) {
      const localeDir = path.join(root, "src", "locales", locale);
      mkdirSync(localeDir, { recursive: true });
      for (const [namespace, messages] of Object.entries(namespaces)) {
        writeFileSync(path.join(localeDir, `${namespace}.json`), JSON.stringify(messages));
      }
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

  it("warns about a missing namespace without failing", () => {
    const fixture = completeFixture();
    delete fixture[ZH_CN].settings;
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("settings");
    expect(result.stderr).toContain("missing namespace");
  });

  it("warns about an extra namespace without failing", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].extra = { surprise: "意外" };
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("extra");
    expect(result.stderr).toContain("extra namespace");
  });

  it("warns about a missing key without failing", () => {
    const fixture = completeFixture();
    delete fixture[ZH_CN].common.second;
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("missing key: second");
  });

  it("warns about an extra key without failing", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.surprise = "意外";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("extra key: surprise");
  });

  it("warns about a translation that drops an interpolation placeholder", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open {{url}} as {{user}}";
    fixture[ZH_CN].common.first = "以 {{user}} 身份打开";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("interpolation placeholder mismatch: first");
  });

  it("warns about a translation that drops numeric Trans tags", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open <1>{{url}}</1>";
    fixture[ZH_CN].common.first = "打开 {{url}}";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("Trans tag structure mismatch: first");
  });

  it("warns about a translation that changes a numeric Trans tag index", () => {
    const fixture = completeFixture();
    fixture.en.common.first = "Open <1>{{url}}</1>";
    fixture[ZH_CN].common.first = "打开 <2>{{url}}</2>";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("Trans tag structure mismatch: first");
  });
});

describe("translated value validation", () => {
  it("warns about an empty translated value without failing", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = "";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("empty translated value: first");
  });

  it("warns about a whitespace-only translated value without failing", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = " \t\n ";
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("empty translated value: first");
  });

  it("warns about a non-string translated value without failing", () => {
    const fixture = completeFixture();
    fixture[ZH_CN].common.first = 42;
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).toContain("advisory");
    expect(result.stderr).toContain(ZH_CN);
    expect(result.stderr).toContain("common");
    expect(result.stderr).toContain("non-string translated value: first");
    expect(result.stderr).not.toContain("matchAll is not a function");
  });
});

/**
 * `en` <-> `pseudo` is the half that still gates, and after making real locales
 * advisory it carries all of the weight — so it needs its own coverage rather
 * than being assumed. Both catalogs are ours: English is authored beside the
 * code and `pseudo` is generated from it, so a mismatch is always the author's
 * to fix in the same change.
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
    expect(result.stderr).toContain("advisory");
  });
});

/**
 * The real-locale pass compares SHAPES, not meaning. These pin the limits so a
 * green run is never mistaken for "translated" — the assumption that this check
 * validates translation quality has already caused two wrong conclusions.
 */
describe("the real-locale pass is structural, not translational", () => {
  it("accepts a catalog copy-pasted wholesale from en", () => {
    const fixture = completeFixture();
    fixture[ZH_CN] = structuredClone(fixture.en);
    const result = runFixture(fixture);

    expect(result.status).toBe(0);
    expect(result.stderr).not.toContain("advisory");
  });

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
