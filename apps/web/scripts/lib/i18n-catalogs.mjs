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

export function realLocaleParityIssues(source, translated, locale) {
  const issues = [];
  for (const namespace of source.keys()) {
    if (!translated.has(namespace)) {
      issues.push({ locale, namespace, type: "missing namespace" });
      continue;
    }
    const sourceMessages = source.get(namespace);
    const translatedMessages = translated.get(namespace);
    for (const key of sourceMessages.keys()) {
      if (!translatedMessages.has(key)) {
        issues.push({ locale, namespace, type: "missing key", key });
      }
    }
    for (const key of translatedMessages.keys()) {
      if (!sourceMessages.has(key)) {
        issues.push({ locale, namespace, type: "extra key", key });
      }
    }
  }
  for (const namespace of translated.keys()) {
    if (!source.has(namespace)) {
      issues.push({ locale, namespace, type: "extra namespace" });
    }
  }
  return issues;
}

export function formatParityIssue(issue) {
  const suffix = issue.key ? `: ${issue.key}` : "";
  return `${issue.locale} / ${issue.namespace}: ${issue.type}${suffix}`;
}
