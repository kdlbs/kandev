const PROVIDER_ID_PATTERN = /^[a-z0-9][a-z0-9._-]*$/;
const CORE_PROVIDER_IDS = new Set(["github", "gitlab", "azure_devops"]);

/** Enforces manifest declaration, canonical IDs, and one active owner per provider. */
export class PluginProviderOwnership {
  private owners = new Map<string, string>();
  private declarations = new Map<string, Set<string>>();

  setDeclarations(pluginId: string, ids: string[]): void {
    this.declarations.set(pluginId, new Set(ids));
  }

  claim(pluginId: string, providerId: string): void {
    if (!PROVIDER_ID_PATTERN.test(providerId)) {
      throw new Error(
        `[plugins] provider "${providerId}" must be a canonical lowercase identifier`,
      );
    }
    if (CORE_PROVIDER_IDS.has(providerId.trim().toLowerCase())) {
      throw new Error(`[plugins] provider "${providerId}" is reserved by the host`);
    }
    const declared = this.declarations.get(pluginId);
    if (declared && !declared.has(providerId)) {
      throw new Error(
        `[plugins] "${pluginId}" does not declare repository provider "${providerId}"`,
      );
    }
    const owner = this.owners.get(providerId);
    if (owner && owner !== pluginId) {
      throw new Error(`[plugins] provider "${providerId}" is already owned by "${owner}"`);
    }
    this.owners.set(providerId, pluginId);
  }

  releasePlugin(pluginId: string): void {
    this.owners.forEach((owner, providerId) => {
      if (owner === pluginId) this.owners.delete(providerId);
    });
    this.declarations.delete(pluginId);
  }
}
