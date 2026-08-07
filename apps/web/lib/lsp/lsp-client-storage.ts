function lspStorageKey(sessionId: string, language: string): string {
  return `kandev-lsp:${sessionId}:${language}`;
}

export function saveLspEnabledState(sessionId: string, language: string): void {
  try {
    localStorage.setItem(lspStorageKey(sessionId, language), "1");
  } catch {}
}

export function clearLspEnabledState(sessionId: string, language: string): void {
  try {
    localStorage.removeItem(lspStorageKey(sessionId, language));
  } catch {}
}

export function isLspEnabledInStorage(sessionId: string, language: string): boolean {
  try {
    return localStorage.getItem(lspStorageKey(sessionId, language)) === "1";
  } catch {
    return false;
  }
}
