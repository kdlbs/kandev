import { describe, expect, it } from "vitest";
import { CLOSE_CODE_STATUS, getLspUnavailableSetupHint, toLspLanguage } from "./lsp-json-rpc";
import { LSP_LANGUAGE_OPTIONS } from "./lsp-language-options";
import { getMonacoLanguagesForLsp } from "./lsp-providers";

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
      reason: "remote executor",
      cause: "unsupported_executor",
    });
  });

  it("surfaces connection capacity as unavailable", () => {
    expect(CLOSE_CODE_STATUS[4005]("")).toEqual({
      state: "unavailable",
      reason: "Too many language servers are active",
      cause: "capacity",
    });
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
  });
});
