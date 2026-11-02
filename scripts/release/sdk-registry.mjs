import { writeFile } from "node:fs/promises";
import { createHash } from "node:crypto";

export const packageName = "@kdlbs/plugin-sdk";
export const registryURL = "https://registry.npmjs.org";
export function publicRegistryURL(raw) {
  const url = new URL(raw);
  if (url.origin !== registryURL || url.username || url.password)
    throw new Error("registry URL must use public npm infrastructure");
  return url;
}
export async function readPackage(version, fetchImpl = fetch) {
  const response = await fetchImpl(`${registryURL}/@kdlbs%2fplugin-sdk`, {
    headers: { accept: "application/json", "cache-control": "no-cache" },
    signal: AbortSignal.timeout(20_000),
  });
  publicRegistryURL(response.url);
  if (!response.headers.get("content-type")?.includes("application/json"))
    throw new Error("registry returned a non-JSON response");
  const text = await response.text();
  if (text.length > 5 * 1024 * 1024)
    throw new Error("registry metadata exceeds size limit");
  let document;
  try {
    document = JSON.parse(text);
  } catch {
    throw new Error("registry returned malformed JSON");
  }
  if (response.status === 404 && document?.error === "Not found")
    return { packageExists: false, version: null, latest: null };
  if (!response.ok)
    throw new Error(`registry read failed with HTTP ${response.status}`);
  if (
    document?.name !== packageName ||
    !document.versions ||
    typeof document.versions !== "object" ||
    Array.isArray(document.versions) ||
    !document["dist-tags"] ||
    typeof document["dist-tags"] !== "object"
  )
    throw new Error("registry returned invalid package metadata");
  const selected = document.versions[version] ?? null;
  if (selected) {
    if (
      selected.name !== packageName ||
      selected.version !== version ||
      !/^sha512-[A-Za-z0-9+/]{86}==$/.test(selected.dist?.integrity)
    )
      throw new Error("registry version has invalid identity or integrity");
    publicRegistryURL(selected.dist.tarball);
  }
  const latest = document["dist-tags"].latest ?? null;
  if (latest !== null && typeof latest !== "string")
    throw new Error("registry latest dist-tag is malformed");
  return { packageExists: true, version: selected, latest };
}
export async function downloadArchive(
  url,
  output,
  integrity,
  fetchImpl = fetch,
) {
  publicRegistryURL(url);
  const response = await fetchImpl(url, {
    signal: AbortSignal.timeout(30_000),
  });
  publicRegistryURL(response.url);
  if (
    !response.ok ||
    /(?:html|json)/i.test(response.headers.get("content-type") ?? "")
  )
    throw new Error(
      `registry archive download failed: HTTP ${response.status}`,
    );
  const bytes = Buffer.from(await response.arrayBuffer());
  if (
    bytes.length === 0 ||
    bytes.length > 25 * 1024 * 1024 ||
    bytes[0] !== 0x1f ||
    bytes[1] !== 0x8b
  )
    throw new Error("registry archive is empty, oversized, or not gzip");
  const actual =
    "sha512-" + createHash("sha512").update(bytes).digest("base64");
  if (actual !== integrity)
    throw new Error("registry archive integrity differs from declared bytes");
  await writeFile(output, bytes);
}
