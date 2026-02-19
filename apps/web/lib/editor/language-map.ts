/**
 * Unified language mapping for all editor implementations.
 * Maps file extensions to language IDs for Monaco, CodeMirror, and lowlight.
 */

type LanguageIds = {
  monaco: string;
  codemirror: string;
  lowlight: string;
};

const LANGUAGE_MAP: Record<string, LanguageIds> = {
  // JavaScript/TypeScript
  ".js": { monaco: "javascript", codemirror: "javascript", lowlight: "javascript" },
  ".mjs": { monaco: "javascript", codemirror: "javascript", lowlight: "javascript" },
  ".cjs": { monaco: "javascript", codemirror: "javascript", lowlight: "javascript" },
  ".jsx": { monaco: "javascript", codemirror: "jsx", lowlight: "javascript" },
  ".ts": { monaco: "typescript", codemirror: "typescript", lowlight: "typescript" },
  ".mts": { monaco: "typescript", codemirror: "typescript", lowlight: "typescript" },
  ".cts": { monaco: "typescript", codemirror: "typescript", lowlight: "typescript" },
  ".tsx": { monaco: "typescript", codemirror: "tsx", lowlight: "typescript" },

  // Python
  ".py": { monaco: "python", codemirror: "python", lowlight: "python" },
  ".pyw": { monaco: "python", codemirror: "python", lowlight: "python" },
  ".pyi": { monaco: "python", codemirror: "python", lowlight: "python" },

  // Go
  ".go": { monaco: "go", codemirror: "go", lowlight: "go" },

  // Rust
  ".rs": { monaco: "rust", codemirror: "rust", lowlight: "rust" },

  // Ruby
  ".rb": { monaco: "ruby", codemirror: "ruby", lowlight: "ruby" },
  ".erb": { monaco: "ruby", codemirror: "ruby", lowlight: "erb" },

  // Java/JVM
  ".java": { monaco: "java", codemirror: "java", lowlight: "java" },
  ".kt": { monaco: "kotlin", codemirror: "kotlin", lowlight: "kotlin" },
  ".kts": { monaco: "kotlin", codemirror: "kotlin", lowlight: "kotlin" },
  ".scala": { monaco: "scala", codemirror: "scala", lowlight: "scala" },
  ".groovy": { monaco: "groovy", codemirror: "groovy", lowlight: "groovy" },

  // Swift
  ".swift": { monaco: "swift", codemirror: "swift", lowlight: "swift" },

  // C/C++
  ".c": { monaco: "c", codemirror: "cpp", lowlight: "c" },
  ".h": { monaco: "c", codemirror: "cpp", lowlight: "c" },
  ".cpp": { monaco: "cpp", codemirror: "cpp", lowlight: "cpp" },
  ".cc": { monaco: "cpp", codemirror: "cpp", lowlight: "cpp" },
  ".cxx": { monaco: "cpp", codemirror: "cpp", lowlight: "cpp" },
  ".hpp": { monaco: "cpp", codemirror: "cpp", lowlight: "cpp" },
  ".hxx": { monaco: "cpp", codemirror: "cpp", lowlight: "cpp" },

  // C#
  ".cs": { monaco: "csharp", codemirror: "csharp", lowlight: "csharp" },

  // PHP
  ".php": { monaco: "php", codemirror: "php", lowlight: "php" },

  // Web
  ".html": { monaco: "html", codemirror: "html", lowlight: "xml" },
  ".htm": { monaco: "html", codemirror: "html", lowlight: "xml" },
  ".css": { monaco: "css", codemirror: "css", lowlight: "css" },
  ".scss": { monaco: "scss", codemirror: "css", lowlight: "scss" },
  ".sass": { monaco: "scss", codemirror: "css", lowlight: "scss" },
  ".less": { monaco: "less", codemirror: "css", lowlight: "less" },

  // Data formats
  ".json": { monaco: "json", codemirror: "json", lowlight: "json" },
  ".jsonc": { monaco: "json", codemirror: "json", lowlight: "json" },
  ".yaml": { monaco: "yaml", codemirror: "yaml", lowlight: "yaml" },
  ".yml": { monaco: "yaml", codemirror: "yaml", lowlight: "yaml" },
  ".xml": { monaco: "xml", codemirror: "html", lowlight: "xml" },
  ".toml": { monaco: "ini", codemirror: "toml", lowlight: "ini" },
  ".ini": { monaco: "ini", codemirror: "ini", lowlight: "ini" },
  ".env": { monaco: "ini", codemirror: "properties", lowlight: "ini" },

  // Markup
  ".md": { monaco: "markdown", codemirror: "markdown", lowlight: "markdown" },
  ".mdx": { monaco: "markdown", codemirror: "markdown", lowlight: "markdown" },

  // Shell
  ".sh": { monaco: "shell", codemirror: "bash", lowlight: "bash" },
  ".bash": { monaco: "shell", codemirror: "bash", lowlight: "bash" },
  ".zsh": { monaco: "shell", codemirror: "bash", lowlight: "bash" },
  ".fish": { monaco: "shell", codemirror: "fish", lowlight: "bash" },

  // SQL
  ".sql": { monaco: "sql", codemirror: "sql", lowlight: "sql" },

  // Docker
  ".dockerfile": { monaco: "dockerfile", codemirror: "dockerfile", lowlight: "dockerfile" },

  // GraphQL
  ".graphql": { monaco: "graphql", codemirror: "graphql", lowlight: "graphql" },
  ".gql": { monaco: "graphql", codemirror: "graphql", lowlight: "graphql" },

  // Vue/Svelte
  ".vue": { monaco: "html", codemirror: "vue", lowlight: "xml" },
  ".svelte": { monaco: "html", codemirror: "svelte", lowlight: "xml" },
};

/**
 * Language name aliases (used when a language string is provided instead of file ext).
 * Maps common language names to Monaco language IDs.
 */
const LANGUAGE_ALIASES: Record<string, string> = {
  javascript: "javascript",
  js: "javascript",
  jsx: "javascript",
  typescript: "typescript",
  ts: "typescript",
  tsx: "typescript",
  python: "python",
  py: "python",
  go: "go",
  golang: "go",
  rust: "rust",
  rs: "rust",
  java: "java",
  kotlin: "kotlin",
  scala: "scala",
  swift: "swift",
  c: "c",
  cpp: "cpp",
  "c++": "cpp",
  csharp: "csharp",
  "c#": "csharp",
  php: "php",
  ruby: "ruby",
  rb: "ruby",
  html: "html",
  css: "css",
  scss: "scss",
  less: "less",
  json: "json",
  yaml: "yaml",
  yml: "yaml",
  xml: "xml",
  markdown: "markdown",
  md: "markdown",
  bash: "shell",
  shell: "shell",
  sh: "shell",
  zsh: "shell",
  sql: "sql",
  dockerfile: "dockerfile",
  graphql: "graphql",
  plaintext: "plaintext",
  text: "plaintext",
};

function getExtension(filePath: string): string {
  const lastSlash = filePath.lastIndexOf("/");
  const name = lastSlash >= 0 ? filePath.slice(lastSlash + 1) : filePath;

  // Handle Dockerfile (no extension)
  if (name.toLowerCase() === "dockerfile") return ".dockerfile";

  const dot = name.lastIndexOf(".");
  if (dot === -1) return "";
  return name.slice(dot).toLowerCase();
}

export function getMonacoLanguage(filePath: string): string {
  const ext = getExtension(filePath);
  return LANGUAGE_MAP[ext]?.monaco ?? "plaintext";
}

export function getMonacoLanguageFromName(language: string): string {
  return LANGUAGE_ALIASES[language.toLowerCase()] ?? "plaintext";
}

export function getLowlightLanguage(filePath: string): string {
  const ext = getExtension(filePath);
  return LANGUAGE_MAP[ext]?.lowlight ?? "plaintext";
}

export function getLowlightLanguageFromName(language: string): string {
  // For lowlight, the language name is typically the same
  const lower = language.toLowerCase();
  // Try to find via alias → then look it up in the map values
  const monacoLang = LANGUAGE_ALIASES[lower];
  if (!monacoLang) return lower;

  // Find a matching entry in the map
  for (const ids of Object.values(LANGUAGE_MAP)) {
    if (ids.monaco === monacoLang) return ids.lowlight;
  }
  return lower;
}
