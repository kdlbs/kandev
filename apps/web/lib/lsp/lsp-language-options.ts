export type LspLanguageOption = {
  id: string;
  label: string;
  binary: string;
  docsUrl: string;
  installHint: string;
  autoInstallSupported: boolean;
};

export const LSP_LANGUAGE_OPTIONS: LspLanguageOption[] = [
  {
    id: "typescript",
    label: "TypeScript / JavaScript",
    binary: "typescript-language-server",
    docsUrl:
      "https://github.com/typescript-language-server/typescript-language-server#workspace-configuration",
    installHint:
      "Installs typescript-language-server and typescript via npm into ~/.kandev/lsp-servers/",
    autoInstallSupported: true,
  },
  {
    id: "go",
    label: "Go",
    binary: "gopls",
    docsUrl: "https://github.com/golang/tools/blob/master/gopls/doc/settings.md",
    installHint: 'Runs "go install golang.org/x/tools/gopls@latest". Requires Go to be installed.',
    autoInstallSupported: true,
  },
  {
    id: "rust",
    label: "Rust",
    binary: "rust-analyzer",
    docsUrl: "https://rust-analyzer.github.io/book/configuration.html",
    installHint:
      "Downloads the rust-analyzer binary from GitHub releases into ~/.kandev/lsp-servers/",
    autoInstallSupported: true,
  },
  {
    id: "python",
    label: "Python",
    binary: "pyright-langserver",
    docsUrl: "https://microsoft.github.io/pyright/#/settings",
    installHint: "Installs pyright via npm into ~/.kandev/lsp-servers/",
    autoInstallSupported: true,
  },
  {
    id: "kotlin",
    label: "Kotlin (experimental)",
    binary: "kotlin-lsp",
    docsUrl: "https://kotlinlang.org/docs/kotlin-lsp.html",
    installHint:
      "Install kotlin-lsp manually on the task host's PATH. For Local Docker tasks, it must be installed and on PATH inside the task container.",
    autoInstallSupported: false,
  },
];
