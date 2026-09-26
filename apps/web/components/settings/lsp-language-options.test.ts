import { afterEach, describe, expect, it } from "vitest";
import { activateLocale, DEFAULT_LOCALE, t } from "@/lib/i18n";
import {
  LSP_LANGUAGE_OPTIONS,
  lspAutoInstallConfigurable,
  lspLanguageDisplayLabel,
} from "./lsp-language-options";

afterEach(async () => activateLocale(DEFAULT_LOCALE));

describe("lspLanguageDisplayLabel", () => {
  it("keeps stable language names verbatim", () => {
    const go = LSP_LANGUAGE_OPTIONS.find((language) => language.id === "go");
    expect(go && lspLanguageDisplayLabel(go, t)).toBe("Go");
  });

  it("localizes the Kotlin experimental qualifier", async () => {
    const kotlin = LSP_LANGUAGE_OPTIONS.find((language) => language.id === "kotlin");
    expect(kotlin && lspLanguageDisplayLabel(kotlin, t)).toBe("Kotlin (experimental)");

    await activateLocale("pseudo");

    expect(kotlin && lspLanguageDisplayLabel(kotlin, t)).toBe("Kotlin (ēxƥēŕĩḿēńţàĺ)");
  });
});

describe("lspAutoInstallConfigurable", () => {
  const rust = LSP_LANGUAGE_OPTIONS.find((language) => language.id === "rust");

  it("uses a task-host-independent preference list", () => {
    expect(rust && lspAutoInstallConfigurable(rust, ["go", "python", "typescript"])).toBe(false);
    expect(rust && lspAutoInstallConfigurable(rust, ["go", "python", "rust", "typescript"])).toBe(
      true,
    );
  });

  it("keeps manual-only languages unavailable even with malformed capabilities", () => {
    const kotlin = LSP_LANGUAGE_OPTIONS.find((language) => language.id === "kotlin");
    expect(kotlin && lspAutoInstallConfigurable(kotlin, ["kotlin"])).toBe(false);
  });
});
