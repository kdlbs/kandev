import { describe, expect, it } from "vitest";
import { resolveCursorMcpDiscoveryReason } from "./cursor-mcp-discovery-reason";

describe("resolveCursorMcpDiscoveryReason", () => {
  it.each([
    ["timeout", "task:agentMcpCursorDiscoveryTimeout", true],
    ["canceled", "task:agentMcpCursorDiscoveryCanceled", false],
    [
      "credential_reader_unavailable",
      "task:agentMcpCursorDiscoveryCredentialReaderUnavailable",
      true,
    ],
    ["credentials_unavailable", "task:agentMcpCursorDiscoveryCredentialsUnavailable", true],
    ["request_invalid", "task:agentMcpCursorDiscoveryRequestInvalid", true],
    ["request_failed", "task:agentMcpCursorDiscoveryRequestFailed", true],
    ["service_status", "task:agentMcpCursorDiscoveryServiceStatus", true],
    ["response_unreadable", "task:agentMcpCursorDiscoveryResponseUnreadable", true],
    ["response_too_large", "task:agentMcpCursorDiscoveryResponseTooLarge", true],
    ["response_invalid", "task:agentMcpCursorDiscoveryResponseInvalid", true],
  ])(
    "maps %s to its localized message and continuation state",
    (failureCode, messageKey, marketplaceLookupFailed) => {
      expect(resolveCursorMcpDiscoveryReason(failureCode)).toEqual({
        messageKey,
        marketplaceLookupFailed,
      });
    },
  );

  it.each([undefined, "unavailable", "unknown_reason", "toString"])(
    "uses general discovery copy for %s",
    (failureCode) => {
      expect(resolveCursorMcpDiscoveryReason(failureCode)).toEqual({
        messageKey: "task:agentMcpCursorDiscoveryFailed",
        marketplaceLookupFailed: false,
      });
    },
  );
});
