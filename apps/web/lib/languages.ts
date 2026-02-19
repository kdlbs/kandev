/**
 * Centralized language utilities for file extension mapping and syntax highlighting.
 */

import { javascript } from "@codemirror/lang-javascript";
import { python } from "@codemirror/lang-python";
import { go } from "@codemirror/lang-go";
import { rust } from "@codemirror/lang-rust";
import { java } from "@codemirror/lang-java";
import { cpp } from "@codemirror/lang-cpp";
import { css } from "@codemirror/lang-css";
import { html } from "@codemirror/lang-html";
import { json } from "@codemirror/lang-json";
import { markdown } from "@codemirror/lang-markdown";
import { yaml } from "@codemirror/lang-yaml";
import type { Extension } from "@codemirror/state";

/**
 * Map of file extensions to language identifiers.
 * Used for syntax highlighting and language detection.
 */
export const EXTENSION_TO_LANGUAGE: Record<string, string> = {
  // JavaScript/TypeScript
  js: "javascript",
  mjs: "javascript",
  cjs: "javascript",
  jsx: "jsx",
  ts: "typescript",
  mts: "typescript",
  cts: "typescript",
  tsx: "tsx",

  // Python
  py: "python",
  pyw: "python",
  pyi: "python",

  // Go
  go: "go",

  // Rust
  rs: "rust",

  // Ruby
  rb: "ruby",
  erb: "ruby",

  // Java/JVM
  java: "java",
  kt: "kotlin",
  kts: "kotlin",
  scala: "scala",
  groovy: "groovy",

  // Swift
  swift: "swift",

  // C/C++
  c: "c",
  h: "c",
  cpp: "cpp",
  cc: "cpp",
  cxx: "cpp",
  hpp: "cpp",
  hxx: "cpp",

  // C#
  cs: "csharp",

  // PHP
  php: "php",

  // Web
  html: "html",
  htm: "html",
  css: "css",
  scss: "scss",
  sass: "sass",
  less: "less",

  // Data formats
  json: "json",
  jsonc: "json",
  yaml: "yaml",
  yml: "yaml",
  xml: "xml",
  toml: "toml",
  ini: "ini",
  env: "properties",

  // Markup
  md: "markdown",
  mdx: "markdown",

  // Shell
  sh: "bash",
  bash: "bash",
  zsh: "bash",
  fish: "fish",

  // SQL
  sql: "sql",

  // Docker
  dockerfile: "dockerfile",

  // GraphQL
  graphql: "graphql",
  gql: "graphql",

  // Misc
  vue: "vue",
  svelte: "svelte",
};

/**
 * Get the language identifier from a file extension.
 * @param ext - File extension (without the dot)
 * @returns Language identifier or 'plaintext' if unknown
 */
export function getLanguageFromExtension(ext: string): string {
  return EXTENSION_TO_LANGUAGE[ext.toLowerCase()] || "plaintext";
}

/**
 * Get the language identifier from a file path.
 * @param filePath - Full or relative file path
 * @returns Language identifier or 'plaintext' if unknown
 */
export function getLanguageFromPath(filePath: string): string {
  const ext = filePath.split(".").pop()?.toLowerCase() || "";
  return getLanguageFromExtension(ext);
}

/**
 * Lookup map from language identifier to CodeMirror extension factory.
 * Each entry maps a language alias to a function that creates the extension.
 */
const CODEMIRROR_EXTENSION_MAP: Record<string, () => Extension> = {
  javascript: () => javascript(),
  js: () => javascript(),
  jsx: () => javascript({ jsx: true }),
  typescript: () => javascript({ typescript: true }),
  ts: () => javascript({ typescript: true }),
  tsx: () => javascript({ jsx: true, typescript: true }),
  python: () => python(),
  py: () => python(),
  go: () => go(),
  golang: () => go(),
  rust: () => rust(),
  rs: () => rust(),
  java: () => java(),
  cpp: () => cpp(),
  "c++": () => cpp(),
  c: () => cpp(),
  cc: () => cpp(),
  cxx: () => cpp(),
  css: () => css(),
  scss: () => css(),
  sass: () => css(),
  less: () => css(),
  html: () => html(),
  htm: () => html(),
  json: () => json(),
  jsonc: () => json(),
  markdown: () => markdown(),
  md: () => markdown(),
  mdx: () => markdown(),
  yaml: () => yaml(),
  yml: () => yaml(),
};

/**
 * Get the CodeMirror language extension for a given language identifier.
 * @param language - Language identifier (e.g., 'typescript', 'python')
 * @returns CodeMirror Extension or undefined if no extension available
 */
export function getCodeMirrorExtension(language: string): Extension | undefined {
  const factory = CODEMIRROR_EXTENSION_MAP[language.toLowerCase()];
  return factory?.();
}

/**
 * Get the CodeMirror language extension directly from a file path.
 * @param filePath - Full or relative file path
 * @returns CodeMirror Extension or undefined if no extension available
 */
export function getCodeMirrorExtensionFromPath(filePath: string): Extension | undefined {
  const language = getLanguageFromPath(filePath);
  return getCodeMirrorExtension(language);
}
