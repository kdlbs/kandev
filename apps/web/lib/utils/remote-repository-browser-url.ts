/**
 * Resolve supported first-party HTTPS clone URLs to their matching browser
 * pages. Plugin providers need an explicit browser URL contract before their
 * clone URL can be used for navigation.
 */
const DIRECT_BROWSER_URL_PROVIDERS = new Set(["github", "gitlab", "azure_devops"]);

export function remoteRepositoryBrowserUrl(
  remoteUrl: string | undefined,
  provider: string,
): string | null {
  if (!DIRECT_BROWSER_URL_PROVIDERS.has(provider)) return null;

  const value = remoteUrl?.trim();
  if (!value || /[\u0000-\u001f\u007f]/.test(value) || !/^https:\/\/[^/?#]+/i.test(value)) {
    return null;
  }

  try {
    const url = new URL(value);
    if (
      url.protocol !== "https:" ||
      !url.hostname ||
      url.username ||
      url.password ||
      value.includes("?") ||
      value.includes("#")
    ) {
      return null;
    }

    const path = url.pathname.replace(/\/+$/, "").replace(/\.git$/i, "");
    url.pathname = path || "/";
    return url.toString();
  } catch {
    return null;
  }
}
