const CURSOR_DISCOVERY_REASON_KEYS: Record<string, string> = {
  timeout: "task:agentMcpCursorDiscoveryTimeout",
  canceled: "task:agentMcpCursorDiscoveryCanceled",
  credential_reader_unavailable: "task:agentMcpCursorDiscoveryCredentialReaderUnavailable",
  credentials_unavailable: "task:agentMcpCursorDiscoveryCredentialsUnavailable",
  request_invalid: "task:agentMcpCursorDiscoveryRequestInvalid",
  request_failed: "task:agentMcpCursorDiscoveryRequestFailed",
  service_status: "task:agentMcpCursorDiscoveryServiceStatus",
  response_unreadable: "task:agentMcpCursorDiscoveryResponseUnreadable",
  response_too_large: "task:agentMcpCursorDiscoveryResponseTooLarge",
  response_invalid: "task:agentMcpCursorDiscoveryResponseInvalid",
};

export function resolveCursorMcpDiscoveryReason(failureCode?: string) {
  if (
    failureCode !== undefined &&
    Object.prototype.hasOwnProperty.call(CURSOR_DISCOVERY_REASON_KEYS, failureCode)
  ) {
    return {
      messageKey: CURSOR_DISCOVERY_REASON_KEYS[failureCode],
      marketplaceLookupFailed: failureCode !== "canceled",
    };
  }
  return {
    messageKey: "task:agentMcpCursorDiscoveryFailed",
    marketplaceLookupFailed: false,
  };
}
