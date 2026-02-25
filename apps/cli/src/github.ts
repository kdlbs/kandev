import crypto from "node:crypto";
import fs from "node:fs";
import https from "node:https";
import path from "node:path";

// Allow overriding the GitHub repo for forks/testing.
const OWNER = process.env.KANDEV_GITHUB_OWNER || "kdlbs";
const REPO = process.env.KANDEV_GITHUB_REPO || "kandev";
const WEB_BASE = `https://github.com/${OWNER}/${REPO}`;

export type ReleaseInfo = {
  tag_name: string;
};

function authHeaders(): Record<string, string> {
  if (process.env.KANDEV_GITHUB_TOKEN) {
    return { Authorization: `Bearer ${process.env.KANDEV_GITHUB_TOKEN}` };
  }
  return {};
}

/**
 * Resolve the latest release tag by following the redirect from
 * github.com/{owner}/{repo}/releases/latest.
 *
 * Uses github.com (not api.github.com) so it is not subject to
 * the REST API rate limit (60 req/hour per IP).
 */
function resolveLatestTag(): Promise<string> {
  const url = `${WEB_BASE}/releases/latest`;
  return new Promise((resolve, reject) => {
    const req = https.get(
      url,
      {
        headers: { "User-Agent": "kandev-npx", ...authHeaders() },
        // Do not follow redirects — we just need the Location header.
        followRedirect: false,
      } as https.RequestOptions,
      (res) => {
        // Drain the response body to free the socket.
        res.resume();

        if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location) {
          // Location: https://github.com/{owner}/{repo}/releases/tag/v0.2
          const match = res.headers.location.match(/\/releases\/tag\/(.+)$/);
          if (match) {
            return resolve(match[1]);
          }
          return reject(new Error(`Could not parse tag from redirect: ${res.headers.location}`));
        }

        // GitHub returns 200 if there's only one release (no redirect).
        // In that case we need to parse the page — but this is uncommon.
        // Fall back to a HEAD request on the resolved URL.
        if (res.statusCode === 200 && res.headers.location) {
          const match = res.headers.location.match(/\/releases\/tag\/(.+)$/);
          if (match) return resolve(match[1]);
        }

        reject(new Error(`Failed to resolve latest release (HTTP ${res.statusCode})`));
      },
    );
    req.setTimeout(5000, () => {
      req.destroy(new Error("Request timed out resolving latest release"));
    });
    req.on("error", reject);
  });
}

/**
 * Verify that a specific release tag exists.
 */
function verifyTagExists(tag: string): Promise<void> {
  const url = `${WEB_BASE}/releases/tag/${tag}`;
  return new Promise((resolve, reject) => {
    const req = https.request(
      url,
      {
        method: "HEAD",
        headers: { "User-Agent": "kandev-npx", ...authHeaders() },
      },
      (res) => {
        res.resume();
        // GitHub returns 200 for the tag page, or 302 redirect to the tag page.
        if (res.statusCode === 200 || res.statusCode === 301 || res.statusCode === 302) {
          return resolve();
        }
        reject(new Error(`Release tag '${tag}' not found (HTTP ${res.statusCode})`));
      },
    );
    req.setTimeout(5000, () => {
      req.destroy(new Error(`Request timed out verifying tag '${tag}'`));
    });
    req.on("error", reject);
    req.end();
  });
}

/**
 * Get release info. Uses github.com web URLs (not api.github.com)
 * to avoid REST API rate limits.
 */
export async function getRelease(version?: string): Promise<ReleaseInfo> {
  if (version) {
    await verifyTagExists(version);
    return { tag_name: version };
  }
  const tag = await resolveLatestTag();
  return { tag_name: tag };
}

// -- Asset downloading --------------------------------------------------------

export function readSha256(pathToSha: string): string | null {
  if (!fs.existsSync(pathToSha)) {
    return null;
  }
  const content = fs.readFileSync(pathToSha, "utf8").trim();
  const first = content.split(/\s+/)[0];
  return first?.toLowerCase() || null;
}

function downloadFile(
  url: string,
  destPath: string,
  expectedSha256?: string | null,
  onProgress?: (downloaded: number, total: number) => void,
): Promise<string> {
  const tempPath = `${destPath}.tmp`;
  return new Promise<string>((resolve, reject) => {
    const file = fs.createWriteStream(tempPath);
    const hash = crypto.createHash("sha256");

    const cleanup = () => {
      try {
        fs.unlinkSync(tempPath);
      } catch {}
    };

    const handleResponse = (res: import("node:http").IncomingMessage) => {
      // Follow redirects (GitHub returns 302 to signed S3/CDN URL).
      // Strip auth header on redirect to avoid S3 rejecting it.
      if ((res.statusCode === 301 || res.statusCode === 302) && res.headers.location) {
        const redirectReq = https.get(
          res.headers.location,
          { headers: { "User-Agent": "kandev-npx" } },
          handleResponse,
        );
        redirectReq.setTimeout(30000, () => {
          redirectReq.destroy(new Error(`Request timed out downloading ${url}`));
        });
        redirectReq.on("error", (err) => {
          file.close();
          cleanup();
          reject(err);
        });
        return;
      }

      if (res.statusCode !== 200) {
        file.close();
        cleanup();
        return reject(new Error(`HTTP ${res.statusCode} downloading ${url}`));
      }
      const totalSize = parseInt(res.headers["content-length"] || "0", 10);
      let downloadedSize = 0;
      res.on("data", (chunk) => {
        downloadedSize += chunk.length;
        hash.update(chunk);
        if (onProgress) onProgress(downloadedSize, totalSize);
      });
      res.pipe(file);
      file.on("finish", () => {
        file.close();
        const actualSha256 = hash.digest("hex");
        if (expectedSha256 && actualSha256 !== expectedSha256) {
          cleanup();
          return reject(
            new Error(`Checksum mismatch: expected ${expectedSha256}, got ${actualSha256}`),
          );
        }
        try {
          fs.renameSync(tempPath, destPath);
          resolve(destPath);
        } catch (err) {
          cleanup();
          reject(err);
        }
      });
    };

    const req = https.get(
      url,
      {
        headers: {
          "User-Agent": "kandev-npx",
          ...authHeaders(),
        },
      },
      handleResponse,
    );
    req.setTimeout(30000, () => {
      req.destroy(new Error(`Request timed out downloading ${url}`));
    });
    req.on("error", (err) => {
      file.close();
      cleanup();
      reject(err);
    });
  });
}

/**
 * Ensure a release asset is downloaded and cached.
 *
 * Downloads directly from github.com/{owner}/{repo}/releases/download/{tag}/{asset}
 * instead of the API, avoiding rate limits.
 */
export async function ensureAsset(
  tag: string,
  assetName: string,
  cacheDir: string,
  onProgress?: (downloaded: number, total: number) => void,
): Promise<string> {
  fs.mkdirSync(cacheDir, { recursive: true });
  const destPath = path.join(cacheDir, assetName);
  const shaPath = `${destPath}.sha256`;

  // Download sha256 checksum if not already cached.
  let expectedSha = readSha256(shaPath);
  if (!expectedSha) {
    const shaUrl = `${WEB_BASE}/releases/download/${tag}/${assetName}.sha256`;
    try {
      await downloadFile(shaUrl, shaPath);
      expectedSha = readSha256(shaPath);
    } catch {
      // sha256 file may not exist for this release — continue without it.
    }
  }

  // Return cached tarball if it exists and checksum matches.
  if (fs.existsSync(destPath)) {
    if (!expectedSha) {
      return destPath;
    }
    const hash = crypto.createHash("sha256");
    const file = fs.readFileSync(destPath);
    hash.update(file);
    if (hash.digest("hex") === expectedSha) {
      return destPath;
    }
    fs.unlinkSync(destPath);
  }

  // Download the asset.
  const assetUrl = `${WEB_BASE}/releases/download/${tag}/${assetName}`;
  await downloadFile(assetUrl, destPath, expectedSha, onProgress);
  return destPath;
}
