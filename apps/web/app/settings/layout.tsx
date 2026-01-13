import { SettingsLayoutClient } from '@/components/settings/settings-layout-client';
import { StateHydrator } from '@/components/state-hydrator';
import { fetchWorkspaces } from '@/lib/ssr/http';

export default function SettingsLayout({
  children,
}: {
  children: React.ReactNode;
}) {
  return (
    <SettingsLayoutServer>{children}</SettingsLayoutServer>
  );
}

async function SettingsLayoutServer({ children }: { children: React.ReactNode }) {
  let initialState = {};
  try {
    const workspaces = await fetchWorkspaces();
    initialState = {
      workspaces: {
        items: workspaces.workspaces.map((workspace) => ({
          id: workspace.id,
          name: workspace.name,
          default_executor_id: workspace.default_executor_id ?? null,
          default_environment_id: workspace.default_environment_id ?? null,
          default_agent_profile_id: workspace.default_agent_profile_id ?? null,
        })),
        activeId: workspaces.workspaces[0]?.id ?? null,
      },
    };
  } catch {
    initialState = {};
  }

  return (
    <>
      <StateHydrator initialState={initialState} />
      <SettingsLayoutClient>{children}</SettingsLayoutClient>
    </>
  );
}
