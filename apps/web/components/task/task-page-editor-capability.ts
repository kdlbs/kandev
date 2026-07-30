type SessionEditorCapabilityStatus = {
  capabilities?: {
    embedded_vscode?: boolean;
  };
};

export function getEmbeddedVscodeSupported(
  status: SessionEditorCapabilityStatus | null | undefined,
): boolean {
  return status?.capabilities?.embedded_vscode === true;
}
