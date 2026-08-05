import { describe, expect, it } from "vitest";
import { activateLocale } from "@/lib/i18n";
import { CLOSE_CODE_STATUS, getLspUnavailableSetupHint, toLspLanguage } from "./lsp-json-rpc";
import { LSP_LANGUAGE_OPTIONS } from "./lsp-language-options";
import { getMonacoLanguagesForLsp } from "./lsp-providers";

const BACKEND_REASON = "backend English";

describe("Kotlin LSP language mapping", () => {
  it("maps Monaco Kotlin documents to the Kotlin language server", () => {
    expect(toLspLanguage("kotlin")).toBe("kotlin");
  });

  it("registers Monaco providers for Kotlin", () => {
    expect(getMonacoLanguagesForLsp("kotlin")).toEqual(["kotlin"]);
  });

  it("marks Kotlin as requiring manual installation", () => {
    expect(LSP_LANGUAGE_OPTIONS.find((language) => language.id === "kotlin")).toMatchObject({
      binary: "kotlin-lsp",
      installHint:
        "Install kotlin-lsp manually on the task host's PATH. For Local Docker tasks, it must be installed and on PATH inside the task container.",
      autoInstallSupported: false,
    });
  });
});

describe("LSP task-host close codes", () => {
  it("surfaces unsupported executors as unavailable", () => {
    expect(CLOSE_CODE_STATUS[4004]("remote executor")).toEqual({
      state: "unavailable",
      reason: "Language servers are not supported by this task executor",
      cause: "unsupported_executor",
    });
  });

  it("surfaces connection capacity as unavailable", () => {
    expect(CLOSE_CODE_STATUS[4005]("active LSP connection cap exceeded")).toEqual({
      state: "unavailable",
      reason: "Too many language servers are active",
      cause: "capacity",
    });
  });

  it("surfaces task-host auto-install limitations separately from a disabled preference", () => {
    expect(CLOSE_CODE_STATUS[4007](BACKEND_REASON)).toEqual({
      state: "unavailable",
      reason: "Auto-install is unavailable on this task host",
      cause: "auto_install_unsupported",
    });
  });

  it("localizes categorical close codes instead of rendering transport prose", async () => {
    await activateLocale("pseudo");
    try {
      expect(CLOSE_CODE_STATUS[4004](BACKEND_REASON)).toEqual({
        state: "unavailable",
        reason: "Ĺàńĝũàĝē śēŕvēŕś àŕē ńōţ śũƥƥōŕţēď ƀŷ ţĥĩś ţàśķ ēxēćũţōŕ",
        cause: "unsupported_executor",
      });
      expect(CLOSE_CODE_STATUS[4005](BACKEND_REASON)).toEqual({
        state: "unavailable",
        reason: "Ţōō ḿàńŷ ĺàńĝũàĝē śēŕvēŕś àŕē àćţĩvē",
        cause: "capacity",
      });
      expect(CLOSE_CODE_STATUS[4006](BACKEND_REASON)).toEqual({
        state: "error",
        reason: "ĺàńĝũàĝē śēŕvēŕ ēxĩţēď",
      });
      expect(CLOSE_CODE_STATUS[4007](BACKEND_REASON)).toEqual({
        state: "unavailable",
        reason: "Àũţō-ĩńśţàĺĺ ĩś ũńàvàĩĺàƀĺē ōń ţĥĩś ţàśķ ĥōśţ",
        cause: "auto_install_unsupported",
      });
    } finally {
      await activateLocale("en");
    }
  });

  it("offers setup help only when the server binary is missing", () => {
    expect(getLspUnavailableSetupHint(CLOSE_CODE_STATUS[4004](""), "kotlin")).toBeNull();
    expect(getLspUnavailableSetupHint(CLOSE_CODE_STATUS[4005](""), "python")).toBeNull();
    expect(getLspUnavailableSetupHint(CLOSE_CODE_STATUS[4001](""), "kotlin")).toBe(
      "Install kotlin-lsp on the task host, then restart the task.",
    );
    expect(getLspUnavailableSetupHint(CLOSE_CODE_STATUS[4001](""), "python")).toBe(
      "Enable auto-install in Settings → Editors.",
    );
    expect(getLspUnavailableSetupHint(CLOSE_CODE_STATUS[4007](""), "rust")).toBe(
      "Install the language server manually on the task host, then retry.",
    );
  });
});
