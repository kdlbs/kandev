/** Matches the task-create picker’s URL-shape check, including scheme-less URLs. */
export function looksLikeURL(value: string): boolean {
  if (!value) return false;
  const withScheme = /^[a-z][a-z\d+.-]*:\/\//i.test(value) ? value : `https://${value}`;
  try {
    const parsed = new URL(withScheme);
    return parsed.hostname.includes(".") && value.includes("/");
  } catch {
    return false;
  }
}

/**
 * Supported provider-host restriction shared by task creation and workspace
 * source attachment. SCP Git syntax is valid only for the same providers.
 */
export function looksLikeSupportedRemoteURL(value: string): boolean {
  if (/^git@(github\.com|gitlab\.com|ssh\.dev\.azure\.com):\S+$/i.test(value)) return true;
  if (!looksLikeURL(value)) return false;
  const candidate = /^[a-z][a-z\d+.-]*:\/\//i.test(value) ? value : `https://${value}`;
  try {
    const host = new URL(candidate).hostname.toLowerCase();
    return host === "github.com" || host === "gitlab.com" || host === "dev.azure.com";
  } catch {
    return false;
  }
}
