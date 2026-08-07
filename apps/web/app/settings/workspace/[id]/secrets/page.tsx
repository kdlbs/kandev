import { SecretsSettings } from "@/components/settings/secrets-settings";
import { listSecrets } from "@/lib/api/domains/secrets-api";
import type { SecretListItem } from "@/lib/types/http-secrets";

export default async function WorkspaceSecretsPage({
  params,
}: {
  params: Promise<{ id: string }>;
}) {
  const { id } = await params;
  let initialItems: SecretListItem[] = [];
  try {
    initialItems = await listSecrets({
      scope: "workspace",
      workspaceId: id,
      cache: "no-store",
    });
  } catch {
    initialItems = [];
  }

  return <SecretsSettings scope="workspace" workspaceId={id} initialItems={initialItems} />;
}
