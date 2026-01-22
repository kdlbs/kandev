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
export function detectPreviewUrl(line: string): PreviewUrlInfo | null {
  // Pattern for full URLs (e.g., http://localhost:3000, https://127.0.0.1:8080)
  const fullUrlPattern = /https?:\/\/(?:localhost|127\.0\.0\.1|0\.0\.0\.0)(?::\d+)?[^\s]*/gi;

  // Pattern for host:port (e.g., localhost:3000, 0.0.0.0:8080)
  const hostPortPattern = /(?:localhost|127\.0\.0\.1|0\.0\.0\.0):(\d{2,5})/gi;

  // Try to match full URLs first
  const fullUrlMatches = line.match(fullUrlPattern);

  if (fullUrlMatches && fullUrlMatches.length > 0) {
    // Filter out localhost URLs without port - they're not valid dev server URLs
    for (const match of fullUrlMatches) {
      try {
        const parsed = new URL(match);
        const isLocalhost = ['localhost', '127.0.0.1', '0.0.0.0'].includes(parsed.hostname);

        // Reject localhost URLs without a port
        if (isLocalhost && !parsed.port) {
          continue; // Try next match or fall through to host:port pattern
        }

        return {
          url: parsed.toString(),
          port: parsed.port ? Number(parsed.port) : undefined,
          scheme: parsed.protocol === 'https:' ? 'https' : 'http',
        };
      } catch {
        // Invalid URL, continue to next match
        continue;
      }
    }
  }

  // Fall back to host:port pattern
  const hostPortMatches = line.match(hostPortPattern);

  if (hostPortMatches && hostPortMatches.length > 0) {
    const match = hostPortMatches[hostPortMatches.length - 1];
    const portMatch = match.match(/:(\d{2,5})$/);
    const port = portMatch ? Number(portMatch[1]) : undefined;

    // Infer scheme from context (look for 'https' in the line)
    const scheme = /https/i.test(line) ? 'https' : 'http';

    return {
      url: `${scheme}://${match}`,
      port,
      scheme,
    };
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
