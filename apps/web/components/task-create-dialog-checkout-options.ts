import type { RepositoryCheckoutOptions } from "@/lib/types/repository-checkout-options";

export function parseCheckoutDirectories(value: string): {
  directories: string[];
  error?: true;
  errorLines?: number[];
} {
  const lines = value.split(/\r?\n/u);
  const directories = [...new Set(lines.filter(Boolean))];
  const errorLines = lines.flatMap((line, index) =>
    line && invalidDirectory(line) ? [index + 1] : [],
  );
  const invalid =
    errorLines.length > 0 ||
    directories.length > 64 ||
    new TextEncoder().encode(JSON.stringify(directories)).length > 16_000;
  return invalid ? { directories, error: true, errorLines } : { directories };
}

function invalidDirectory(directory: string): boolean {
  return (
    new TextEncoder().encode(directory).length > 4096 ||
    /[\\:*?[\]{}\u0000-\u001f\u007f]/u.test(directory) ||
    directory.split("/").some((part) => !part || part === "." || part === "..")
  );
}

export function hasCustomCheckoutOptions(options?: RepositoryCheckoutOptions): boolean {
  return (
    !!options && (options.download_mode !== "standard" || options.sparse_directories.length > 0)
  );
}
