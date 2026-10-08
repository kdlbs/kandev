import { expect } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

// The mock agent mirrors Claude Code: after each turn it forwards this
// prompt_suggestion through the _claude/sdkMessage extension notification.
export const NATIVE_SUGGESTION = "Yes, run the tests";

export async function setPromptSuggestions(apiClient: ApiClient, main: boolean, fallback: boolean) {
  const response = await apiClient.rawRequest("PATCH", "/api/v1/user/settings", {
    prompt_suggestions: main,
    prompt_suggestions_fallback: fallback,
  });
  expect(response.ok).toBe(true);
}
