const NAVIGATION_ATTRIBUTES = new Set([
  "href",
  "xlink:href",
  "action",
  "formaction",
  "ping",
  "cite",
  "download",
  "target",
  "srcdoc",
]);

const RESOURCE_ATTRIBUTES = new Set(["src", "poster"]);

const SAFE_ATTRIBUTE_NAMES =
  /^(?:id|class|title|role|lang|dir|name|value|type|alt|width|height|size|rows|cols|rowspan|colspan|for|tabindex|placeholder|checked|selected|disabled|readonly|multiple|open|hidden|kind|label|datetime|scope|viewbox|fill|stroke|stroke-width|d|cx|cy|r|rx|ry|x|x1|x2|y|y1|y2|points|preserveaspectratio|xmlns|xmlns:xlink|data-[\w:.-]+|aria-[\w:.-]+)$/i;

const RESOURCE_TAGS = new Set(["img", "audio", "video", "source", "track"]);

export function isAllowedPreviewResourceUrl(
  value: string,
  ownedBlobTokens: ReadonlySet<string>,
): boolean {
  const normalized = value.trim();
  if (/^data:/i.test(normalized)) return true;
  return /^blob:/i.test(normalized) && ownedBlobTokens.has(normalized);
}

export function filterPreviewSrcSet(value: string, ownedBlobTokens: ReadonlySet<string>): string {
  const candidates = value.match(/(?:data:[^\s]+|blob:[^\s,]+|[^,\s]+)(?:\s+[0-9]+[wx])?/gi) ?? [];
  return candidates
    .map((candidate) => candidate.trim())
    .filter((candidate) => {
      const url = candidate.split(/\s+/, 1)[0];
      return isAllowedPreviewResourceUrl(url, ownedBlobTokens);
    })
    .join(", ");
}

export function sanitizePreviewCss(value: string, ownedBlobTokens: ReadonlySet<string>): string {
  const withoutImports = value
    .replace(/@import\s+[^;{}]+;?/gi, "")
    .replace(/(?:expression|behavior|-moz-binding)\s*:[^;{}]+;?/gi, "");

  return withoutImports.replace(
    /url\(\s*(?:"([^"]*)"|'([^']*)'|([^\s)]+))\s*\)/gi,
    (
      _match,
      doubleQuoted: string | undefined,
      singleQuoted: string | undefined,
      bare: string | undefined,
    ) => {
      const url = doubleQuoted ?? singleQuoted ?? bare ?? "";
      return isAllowedPreviewResourceUrl(url, ownedBlobTokens) ? `url("${url}")` : "none";
    },
  );
}

function isRenderableResourceAttribute(tagName: string, attributeName: string): boolean {
  return RESOURCE_ATTRIBUTES.has(attributeName) && RESOURCE_TAGS.has(tagName);
}

export function sanitizePreviewAttributes(
  tagName: string,
  attributes: Record<string, string>,
  ownedBlobTokens: ReadonlySet<string>,
): Record<string, string> {
  const safe: Record<string, string> = {};
  const normalizedTagName = tagName.toLowerCase();

  for (const [rawName, rawValue] of Object.entries(attributes)) {
    const name = rawName.toLowerCase();
    if (name.startsWith("on") || NAVIGATION_ATTRIBUTES.has(name)) continue;

    if (name === "style") {
      const css = sanitizePreviewCss(rawValue, ownedBlobTokens);
      if (css) safe.style = css;
      continue;
    }

    if (name === "srcset") {
      const srcSet = filterPreviewSrcSet(rawValue, ownedBlobTokens);
      if (srcSet) safe.srcset = srcSet;
      continue;
    }

    if (isRenderableResourceAttribute(normalizedTagName, name)) {
      if (isAllowedPreviewResourceUrl(rawValue, ownedBlobTokens)) safe[name] = rawValue.trim();
      continue;
    }

    if (SAFE_ATTRIBUTE_NAMES.test(name)) safe[name] = rawValue;
  }

  return safe;
}
