export function buildCursorCloudProfileConfig(
  baseConfig: Record<string, string>,
  secretId: string | null,
  callbackUrl: string,
): Record<string, string> {
  const config = { ...baseConfig };
  const normalizedSecretId = secretId?.trim();
  const normalizedCallbackUrl = callbackUrl.trim();
  if (normalizedSecretId) config.cursor_cloud_api_key_secret_id = normalizedSecretId;
  else delete config.cursor_cloud_api_key_secret_id;
  if (normalizedCallbackUrl) config.cursor_cloud_callback_url = normalizedCallbackUrl;
  else delete config.cursor_cloud_callback_url;
  return config;
}

export function hasCursorCloudProfileConfiguration(secretId: string | null, callbackUrl: string) {
  return Boolean(secretId?.trim() && callbackUrl.trim());
}
