import { dwell } from "./causal-waits";

const appStatePath = "/api/v1/app-state?path=%2Fsettings%2Fagents";
const MAX_STARTUP_RETRIES = 10;
const STARTUP_RETRY_DELAY_MS = 500;

export async function loadInterimSettingsInterlockToken(baseUrl: string): Promise<string> {
  for (let attempt = 0; attempt < MAX_STARTUP_RETRIES; attempt++) {
    const response = await fetch(`${baseUrl}${appStatePath}`);
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

  throw new Error("Unable to load E2E settings interlock after backend startup retries");
}
