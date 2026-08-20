import {
  PluginErrorBoundary,
  PluginRouteFallback,
} from "@/components/plugins/plugin-error-boundary";
import { SettingsSection } from "@/components/settings/settings-section";
import { resolvePluginIcon } from "@/lib/plugins/icons";
import { pluginRegistry } from "@/lib/plugins/registry";

/** Renders one lifecycle-owned contribution inside Kandev's native settings section. */
export function renderPluginIntegrationSettings(integrationId: string, workspaceId?: string) {
  const registration = pluginRegistry.getIntegrationSetting(integrationId);
  if (!registration) return null;
  const Component = registration.Component;
  const Action = registration.action;
  const Icon = resolvePluginIcon(registration.icon);
  return (
    <PluginErrorBoundary
      context={`integration settings "${integrationId}"`}
      fallback={<PluginRouteFallback />}
    >
      <SettingsSection
        title={registration.label}
        description={registration.description}
        icon={<Icon className="h-5 w-5" />}
        action={Action ? <Action workspaceId={workspaceId} surface="detail" /> : undefined}
      >
        <Component workspaceId={workspaceId} />
      </SettingsSection>
    </PluginErrorBoundary>
  );
}
