import { dwell } from "./causal-waits";

const appStatePath = "/api/v1/app-state?path=%2Fsettings%2Fagents";
// The backend serves a temporary bootstrap response while it restores the
// database. CI can spend tens of seconds in that window when many shards
// start together, so keep the bounded retry longer than the normal readiness
// probe without waiting forever on a genuinely unhealthy server.
const MAX_STARTUP_RETRIES = 120;
const STARTUP_RETRY_DELAY_MS = 500;

export async function loadInterimSettingsInterlockToken(baseUrl: string): Promise<string> {
  let lastError: unknown;
  for (let attempt = 0; attempt < MAX_STARTUP_RETRIES; attempt++) {
    let response: Response;
    try {
      response = await fetch(`${baseUrl}${appStatePath}`);
    } catch (error) {
      lastError = error;
      if (attempt === MAX_STARTUP_RETRIES - 1) {
        throw new Error("Unable to load E2E settings interlock after backend startup retries", {
          cause: error,
        });
      }
      await dwell(
        STARTUP_RETRY_DELAY_MS,
        "poll-interval",
        "backend startup before the settings interlock is reachable",
      );
      continue;
    }
    if (response.ok) {
      const payload = (await response.json()) as { interimSettingsInterlockToken?: unknown };
      if (
        typeof payload.interimSettingsInterlockToken !== "string" ||
        !payload.interimSettingsInterlockToken
      ) {
        throw new Error("E2E settings interlock token missing from boot payload");
      }
      return payload.interimSettingsInterlockToken;
    }

    if (response.status !== 503 || attempt === MAX_STARTUP_RETRIES - 1) {
      throw new Error(`Unable to load E2E settings interlock (${response.status})`);
    }

    await dwell(
      STARTUP_RETRY_DELAY_MS,
      "poll-interval",
      "backend startup before the settings interlock is available",
    );
  }

  throw new Error("Unable to load E2E settings interlock after backend startup retries", {
    cause: lastError,
  });
}
