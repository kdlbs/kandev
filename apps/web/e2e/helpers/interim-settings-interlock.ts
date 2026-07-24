const appStatePath = "/api/v1/app-state?path=%2Fsettings%2Fagents";

export async function loadInterimSettingsInterlockToken(baseUrl: string): Promise<string> {
  const response = await fetch(`${baseUrl}${appStatePath}`);
  if (!response.ok) {
    throw new Error(`Unable to load E2E settings interlock (${response.status})`);
  }
  const payload = (await response.json()) as { interimSettingsInterlockToken?: unknown };
  if (
    typeof payload.interimSettingsInterlockToken !== "string" ||
    !payload.interimSettingsInterlockToken
  ) {
    throw new Error("E2E settings interlock token missing from boot payload");
  }
  return payload.interimSettingsInterlockToken;
}
