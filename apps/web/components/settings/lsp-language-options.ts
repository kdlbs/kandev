/**
 * The language servers Settings → General → Editors can auto-start.
 *
 * Almost everything here is data rather than copy. `id` keys the auto-start and
 * auto-install sets, the server-config map, and `LSP_DEFAULT_CONFIGS` — a
 * sentinel, never translated. `label` is a language name, `binary` an executable
 * name, and `docsUrl` a URL, so all three are shown verbatim.
 *
 * Only `installHintKey` is prose, and it is a catalog KEY: this table is
 * evaluated once at import, so a `t()` here would resolve before a locale is
 * active and freeze on the boot locale. The package names, commands and paths
 * the hint mentions are supplied separately as `installHintValues`, so no
 * identifier ends up inside translator-controlled text — a translated install
 * path or `go install` command would name something that does not exist.
 */

/** Where the auto-installer puts servers — a real path, shown verbatim. */
const LSP_SERVERS_DIR = "~/.kandev/lsp-servers/";

export type LspLanguageOption = {
  id: string;
  label: string;
  binary: string;
  docsUrl: string;
  installHintKey: string;
  installHintValues: Record<string, string>;
};

export const LSP_LANGUAGE_OPTIONS: LspLanguageOption[] = [
  {
    id: "typescript",
    label: "TypeScript / JavaScript",
    binary: "typescript-language-server",
    docsUrl:
      "https://github.com/typescript-language-server/typescript-language-server#workspace-configuration",
    installHintKey: "settings:lspInstallHintTypescript",
    installHintValues: {
      server: "typescript-language-server",
      runtime: "typescript",
      manager: "npm",
      dir: LSP_SERVERS_DIR,
    },
  },
  {
    id: "go",
    label: "Go",
    binary: "gopls",
    docsUrl: "https://github.com/golang/tools/blob/master/gopls/doc/settings.md",
    installHintKey: "settings:lspInstallHintGo",
    installHintValues: { command: "go install golang.org/x/tools/gopls@latest" },
  },
  {
    id: "rust",
    label: "Rust",
    binary: "rust-analyzer",
    docsUrl: "https://rust-analyzer.github.io/book/configuration.html",
    installHintKey: "settings:lspInstallHintRust",
    installHintValues: { binary: "rust-analyzer", dir: LSP_SERVERS_DIR },
  },
  {
    id: "python",
    label: "Python",
    binary: "pyright-langserver",
    docsUrl: "https://microsoft.github.io/pyright/#/settings",
    installHintKey: "settings:lspInstallHintPython",
    installHintValues: { package: "pyright", manager: "npm", dir: LSP_SERVERS_DIR },
  },
];
