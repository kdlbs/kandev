/**
 * Detects and validates dev server URLs from process output logs.
 * Handles various formats including full URLs and host:port patterns.
 */

export interface PreviewUrlInfo {
  url: string;
  port?: number;
  scheme: 'http' | 'https';
}

/**
 * Detects a preview URL from a line of process output.
 *
 * Rules:
 * - Rejects localhost URLs without a port (e.g., http://localhost)
 * - Accepts full URLs with ports (e.g., http://localhost:3000)
 * - Accepts host:port patterns (e.g., localhost:3000)
 * - Supports localhost, 127.0.0.1, and 0.0.0.0
 *
 * @param line - A line of process output to scan
 * @returns PreviewUrlInfo if a valid URL is found, null otherwise
 */
const LOCALHOST_HOSTS = new Set(['localhost', '127.0.0.1', '0.0.0.0']);

/** Try to parse a full URL match into a PreviewUrlInfo, returning null if invalid. */
function tryParseFullUrl(match: string): PreviewUrlInfo | null {
  try {
    const parsed = new URL(match);
    if (LOCALHOST_HOSTS.has(parsed.hostname) && !parsed.port) return null;
    return {
      url: parsed.toString(),
      port: parsed.port ? Number(parsed.port) : undefined,
      scheme: parsed.protocol === 'https:' ? 'https' : 'http',
    };
  } catch {
    return null;
  }
}

/** Try to extract a URL from host:port pattern matches. */
function tryParseHostPort(line: string, matches: RegExpMatchArray): PreviewUrlInfo | null {
  const match = matches[matches.length - 1];
  const portMatch = match.match(/:(\d{2,5})$/);
  const port = portMatch ? Number(portMatch[1]) : undefined;
  const scheme = /https/i.test(line) ? 'https' : 'http';
  return { url: `${scheme}://${match}`, port, scheme };
}

export function detectPreviewUrl(line: string): PreviewUrlInfo | null {
  const fullUrlPattern = /https?:\/\/(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::\d+)?[^\s]*/gi;
  const hostPortPattern = /(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d{2,5})/gi;

  const fullUrlMatches = line.match(fullUrlPattern);
  if (fullUrlMatches) {
    for (const match of fullUrlMatches) {
      const result = tryParseFullUrl(match);
      if (result) return result;
    }
  }

  const hostPortMatches = line.match(hostPortPattern);
  if (hostPortMatches && hostPortMatches.length > 0) {
    return tryParseHostPort(line, hostPortMatches);
  }

  return null;
}

/**
 * Scans process output for dev server URLs.
 * Returns the last valid URL found.
 *
 * @param output - The full process output to scan
 * @returns The URL string if found, null otherwise
 */
export function detectPreviewUrlFromOutput(output: string): string | null {
  if (!output) return null;

  const lines = output.split('\n');
  let lastValidUrl: string | null = null;

  for (const line of lines) {
    const urlInfo = detectPreviewUrl(line);
    if (urlInfo) {
      lastValidUrl = urlInfo.url;
    }
  }

  return lastValidUrl;
}
