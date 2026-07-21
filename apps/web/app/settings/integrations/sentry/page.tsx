import { SentryIntegrationPage } from "@/components/sentry/sentry-settings";

type IntegrationsSentryPageProps = {
  workspaceId?: string;
};

export default function IntegrationsSentryPage({ workspaceId }: IntegrationsSentryPageProps = {}) {
  return <SentryIntegrationPage workspaceId={workspaceId} />;
}
