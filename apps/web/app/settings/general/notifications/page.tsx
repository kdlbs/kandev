import { NotificationsSettings } from '@/components/settings/notifications-settings';
import { listNotificationProviders } from '@/lib/http';
import type { NotificationProvider } from '@/lib/types/http';

export default async function GeneralNotificationsPage() {
  let providers: NotificationProvider[] | undefined = [];
  let events: string[] = [];
  let appriseAvailable = true;
  try {
    const response = await listNotificationProviders({ cache: 'no-store' });
    providers = response.providers ?? [];
    events = response.events ?? [];
    appriseAvailable = response.apprise_available ?? true;
  } catch {
    providers = undefined;
    events = [];
    appriseAvailable = true;
  }

  return (
    <NotificationsSettings
      initialProviders={providers}
      initialEvents={events}
      initialAppriseAvailable={appriseAvailable}
    />
  );
}
