function lspStorageKey(sessionId: string, language: string): string {
  return `kandev-lsp:${sessionId}:${language}`;
}

function lspLeaseStorageKey(sessionId: string, language: string): string {
  return `kandev-lsp-lease:${sessionId}:${language}`;
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

export function saveLspLeaseHint(sessionId: string, language: string, leaseId: string): void {
  try {
    sessionStorage.setItem(lspLeaseStorageKey(sessionId, language), leaseId);
  } catch {}
}

export function clearLspLeaseHint(sessionId: string, language: string): void {
  try {
    sessionStorage.removeItem(lspLeaseStorageKey(sessionId, language));
  } catch {}
}

export function getLspLeaseHint(sessionId: string, language: string): string | null {
  try {
    return sessionStorage.getItem(lspLeaseStorageKey(sessionId, language));
  } catch {
    return null;
  }
}
