import crypto from "node:crypto";
import fs from "node:fs";
import https from "node:https";
import path from "node:path";

import { CACHE_DIR } from "./constants";

// Allow overriding the GitHub repo for forks/testing.
const OWNER = process.env.KANDEV_GITHUB_OWNER || "kdlbs";
const REPO = process.env.KANDEV_GITHUB_REPO || "kandev";
const API_BASE = `https://api.github.com/repos/${OWNER}/${REPO}`;

type ReleaseAsset = {
  name: string;
  url: string;
  browser_download_url: string;
};

type ReleaseResponse = {
  tag_name?: string;
  assets?: ReleaseAsset[];
};

function requestJson<T>(url: string): Promise<T> {
  return new Promise((resolve, reject) => {
    const req = https.get(
      url,
      {
        headers: {
          "User-Agent": "kandev-npx",
          Accept: "application/vnd.github+json",
          ...(process.env.KANDEV_GITHUB_TOKEN
            ? { Authorization: `Bearer ${process.env.KANDEV_GITHUB_TOKEN}` }
            : {}),
        },
      },
      (res) => {
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} fetching ${url}`));
        }
        let body = "";
        res.on("data", (chunk) => (body += chunk));
        res.on("end", () => {
          try {
            resolve(JSON.parse(body) as T);
          } catch {
            reject(new Error(`Failed to parse JSON from ${url}`));
          }
        });
      },
    );
    req.setTimeout(5000, () => {
      req.destroy(new Error(`Request timed out fetching ${url}`));
    });
    req.on("error", reject);
  });
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
      // Follow redirects (GitHub API returns 302 to signed S3 URL).
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
          Accept: "application/octet-stream",
          ...(process.env.KANDEV_GITHUB_TOKEN
            ? { Authorization: `Bearer ${process.env.KANDEV_GITHUB_TOKEN}` }
            : {}),
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

function findAsset(release: ReleaseResponse, name: string): ReleaseAsset | undefined {
  return release.assets?.find((asset) => asset.name === name);
}

function readSha256(pathToSha: string): string | null {
  if (!fs.existsSync(pathToSha)) {
    return null;
  }
  const content = fs.readFileSync(pathToSha, "utf8").trim();
  const first = content.split(/\s+/)[0];
  return first || null;
}

export async function getRelease(version?: string): Promise<ReleaseResponse> {
  if (version) {
    return requestJson<ReleaseResponse>(`${API_BASE}/releases/tags/${version}`);
  }
  return requestJson<ReleaseResponse>(`${API_BASE}/releases/latest`);
}

export async function ensureAsset(
  release: ReleaseResponse,
  assetName: string,
  cacheDir: string,
  onProgress?: (downloaded: number, total: number) => void,
): Promise<string> {
  const asset = findAsset(release, assetName);
  if (!asset) {
    throw new Error(`Release asset not found: ${assetName}`);
  }

  fs.mkdirSync(cacheDir, { recursive: true });
  const destPath = path.join(cacheDir, assetName);
  const shaPath = `${destPath}.sha256`;

  let expectedSha = readSha256(shaPath);
  if (!expectedSha) {
    const shaAsset = findAsset(release, `${assetName}.sha256`);
    if (shaAsset) {
      await downloadFile(shaAsset.url, shaPath);
      expectedSha = readSha256(shaPath);
    }
  }

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

  await downloadFile(asset.url, destPath, expectedSha, onProgress);
  return destPath;
}
