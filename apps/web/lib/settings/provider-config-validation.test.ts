import { describe, expect, it } from "vitest";
import {
  clearedProviderFields,
  isOpenAICompatibleProvider,
  isValidProviderBaseUrl,
  providerConfigInvalidReasonKey,
} from "@/lib/settings/provider-config-validation";

describe("isValidProviderBaseUrl", () => {
  it("accepts absolute http(s) URLs with a host", () => {
    expect(isValidProviderBaseUrl("http://localhost:20128/v1")).toBe(true);
    expect(isValidProviderBaseUrl("https://router.example.com")).toBe(true);
    expect(isValidProviderBaseUrl("  http://h/v1  ")).toBe(true);
  });

  it("rejects empty, relative, non-http, and hostless values", () => {
    expect(isValidProviderBaseUrl("")).toBe(false);
    expect(isValidProviderBaseUrl("   ")).toBe(false);
    expect(isValidProviderBaseUrl("/v1")).toBe(false);
    expect(isValidProviderBaseUrl("localhost:20128")).toBe(false);
    expect(isValidProviderBaseUrl("ftp://h/v1")).toBe(false);
    expect(isValidProviderBaseUrl("not a url")).toBe(false);
  });
});

describe("providerConfigInvalidReasonKey", () => {
  it("passes native profiles regardless of the other fields", () => {
    expect(providerConfigInvalidReasonKey({ providerKind: "", model: "a/b" })).toBeUndefined();
    expect(providerConfigInvalidReasonKey({})).toBeUndefined();
  });

  it("blocks an openai_compatible profile with a bad base URL", () => {
    expect(
      providerConfigInvalidReasonKey({ providerKind: "openai_compatible", providerBaseUrl: "" }),
    ).toBe("agents:providerBaseUrlInvalid");
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "/relative",
      }),
    ).toBe("agents:providerBaseUrlInvalid");
  });

  it("blocks an openai_compatible profile with a slash in the model", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "http://localhost:20128/v1",
        model: "anthropic/claude",
      }),
    ).toBe("agents:providerModelSlash");
  });

  it("passes a well-formed openai_compatible profile", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "http://localhost:20128/v1",
        model: "gpt-5",
      }),
    ).toBeUndefined();
  });
});

describe("helpers", () => {
  it("isOpenAICompatibleProvider recognizes only the exact kind", () => {
    expect(isOpenAICompatibleProvider("openai_compatible")).toBe(true);
    expect(isOpenAICompatibleProvider("")).toBe(false);
    expect(isOpenAICompatibleProvider("native")).toBe(false);
  });

  it("clearedProviderFields blanks every provider field", () => {
    expect(clearedProviderFields()).toEqual({
      providerKind: "",
      providerBaseUrl: "",
      providerApiKeySecretId: "",
    });
  });
});
