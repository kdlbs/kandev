import fs from "node:fs";
import path from "node:path";

export function readLocaleNamespaces(localesDir, locale) {
  const dir = path.join(localesDir, locale);
  const namespaces = new Map();
  if (!fs.existsSync(dir)) return namespaces;
  for (const file of fs
    .readdirSync(dir)
    .filter((entry) => entry.endsWith(".json"))
    .sort()) {
    const namespace = file.replace(/\.json$/, "");
    const messages = JSON.parse(fs.readFileSync(path.join(dir, file), "utf8"));
    namespaces.set(namespace, new Map(Object.entries(messages)));
  }
  return namespaces;
}

export function discoverRealLocales(localesDir) {
  if (!fs.existsSync(localesDir)) return [];
  return fs
    .readdirSync(localesDir, { withFileTypes: true })
    .filter((entry) => entry.isDirectory() && !["en", "pseudo"].includes(entry.name))
    .map((entry) => entry.name)
    .sort();
}

function interpolationPlaceholders(message) {
  return [
    ...new Set([...message.matchAll(/\{\{\s*-?\s*([^},\s]+)[^}]*\}\}/g)].map((match) => match[1])),
  ].sort();
}

function numericTagStructure(message) {
  const stack = [];
  const nodes = [];
  for (const match of message.matchAll(/<(\/)?(\d+)>/g)) {
    const closing = Boolean(match[1]);
    const tag = match[2];
    if (closing) {
      if (stack.at(-1) !== tag) return null;
      stack.pop();
      continue;
    }
    nodes.push([...stack, tag].join("/"));
    stack.push(tag);
  }
  return stack.length === 0 ? nodes.sort() : null;
}

function sameItems(left, right) {
  return left !== null && right !== null && JSON.stringify(left) === JSON.stringify(right);
}

function translatedValueIssue(value, context) {
  if (typeof value !== "string") {
    return { ...context, type: "non-string translated value" };
  }
  if (value.trim().length === 0) {
    return { ...context, type: "empty translated value" };
  }
  return null;
}

function messageParityIssues(sourceMessage, translatedMessage, context) {
  const issues = [];
  if (
    !sameItems(
      interpolationPlaceholders(sourceMessage),
      interpolationPlaceholders(translatedMessage),
    )
  ) {
    issues.push({ ...context, type: "interpolation placeholder mismatch" });
  }
  if (!sameItems(numericTagStructure(sourceMessage), numericTagStructure(translatedMessage))) {
    issues.push({ ...context, type: "Trans tag structure mismatch" });
  }
  return issues;
}

function translatedValueIssues(translated, locale) {
  const issues = [];
  for (const [namespace, translatedMessages] of translated) {
    for (const [key, value] of translatedMessages) {
      const valueIssue = translatedValueIssue(value, { locale, namespace, key });
      if (valueIssue) issues.push(valueIssue);
    }
  }
  return issues;
}

function namespaceParityIssues(source, translated, locale) {
  const issues = [];
  for (const namespace of source.keys()) {
    if (!translated.has(namespace)) {
      issues.push({ locale, namespace, type: "missing namespace" });
      continue;
    }
    const sourceMessages = source.get(namespace);
    const translatedMessages = translated.get(namespace);
    issues.push(
      ...messageParityIssuesForNamespace(sourceMessages, translatedMessages, locale, namespace),
    );
  }
  return issues;
}

function messageParityIssuesForNamespace(sourceMessages, translatedMessages, locale, namespace) {
  const issues = [];
  for (const key of sourceMessages.keys()) {
    if (!translatedMessages.has(key)) {
      issues.push({ locale, namespace, type: "missing key", key });
      continue;
    }
    const sourceMessage = sourceMessages.get(key);
    const translatedMessage = translatedMessages.get(key);
    if (typeof sourceMessage !== "string" || typeof translatedMessage !== "string") continue;
    issues.push(
      ...messageParityIssues(sourceMessage, translatedMessage, { locale, namespace, key }),
    );
  }
  for (const key of translatedMessages.keys()) {
    if (!sourceMessages.has(key)) {
      issues.push({ locale, namespace, type: "extra key", key });
    }
  }
  return issues;
}

function extraNamespaceIssues(source, translated, locale) {
  const issues = [];
  for (const namespace of translated.keys()) {
    if (!source.has(namespace)) {
      issues.push({ locale, namespace, type: "extra namespace" });
    }
  }
  return issues;
}

export function realLocaleParityIssues(source, translated, locale) {
  return [
    ...translatedValueIssues(translated, locale),
    ...namespaceParityIssues(source, translated, locale),
    ...extraNamespaceIssues(source, translated, locale),
  ];
}

export function formatParityIssue(issue) {
  const suffix = issue.key ? `: ${issue.key}` : "";
  return `${issue.locale} / ${issue.namespace}: ${issue.type}${suffix}`;
}
