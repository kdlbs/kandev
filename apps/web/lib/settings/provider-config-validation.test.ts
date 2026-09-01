import { describe, expect, it } from "vitest";
import {
  clearedProviderFields,
  isCredentialTransportSafe,
  isOpenAICompatibleProvider,
  isValidProviderBaseUrl,
  providerConfigInvalidReasonKey,
} from "@/lib/settings/provider-config-validation";

const BASE_URL_INVALID_KEY = "agents:providerBaseUrlInvalid";
const LOOPBACK_URL = "http://localhost:20128/v1";

describe("isValidProviderBaseUrl", () => {
  it("accepts absolute http(s) URLs with a host", () => {
    expect(isValidProviderBaseUrl(LOOPBACK_URL)).toBe(true);
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
    ).toBe(BASE_URL_INVALID_KEY);
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "/relative",
      }),
    ).toBe(BASE_URL_INVALID_KEY);
  });

  it("blocks an openai_compatible profile with a slash in the model", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: LOOPBACK_URL,
        model: "anthropic/claude",
      }),
    ).toBe("agents:providerModelSlash");
  });

  it("blocks a credentialed openai_compatible profile on a cleartext non-loopback URL", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "http://router.example/v1",
        providerApiKeySecretId: "secret-1",
        model: "gpt-5",
      }),
    ).toBe(BASE_URL_INVALID_KEY);
  });

  it("allows a credentialed profile on https or a loopback URL", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: "https://router.example/v1",
        providerApiKeySecretId: "secret-1",
        model: "gpt-5",
      }),
    ).toBeUndefined();
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: LOOPBACK_URL,
        providerApiKeySecretId: "secret-1",
        model: "gpt-5",
      }),
    ).toBeUndefined();
  });

  it("passes a well-formed openai_compatible profile", () => {
    expect(
      providerConfigInvalidReasonKey({
        providerKind: "openai_compatible",
        providerBaseUrl: LOOPBACK_URL,
        model: "gpt-5",
      }),
    ).toBeUndefined();
  });
});

describe("isCredentialTransportSafe", () => {
  it("accepts https anywhere and http only to loopback", () => {
    expect(isCredentialTransportSafe("https://router.example/v1")).toBe(true);
    expect(isCredentialTransportSafe(LOOPBACK_URL)).toBe(true);
    expect(isCredentialTransportSafe("http://127.0.0.1/v1")).toBe(true);
    expect(isCredentialTransportSafe("http://[::1]:8080/v1")).toBe(true);
  });

  it("rejects cleartext http to a non-loopback host", () => {
    expect(isCredentialTransportSafe("http://router.example/v1")).toBe(false);
    expect(isCredentialTransportSafe("http://10.0.0.4:9000/v1")).toBe(false);
    expect(isCredentialTransportSafe("http://host.docker.internal/v1")).toBe(false);
    expect(isCredentialTransportSafe("not a url")).toBe(false);
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
