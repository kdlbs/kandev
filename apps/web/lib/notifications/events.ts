export const NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT =
  'session.waiting_for_input';

export const DEFAULT_NOTIFICATION_EVENTS = [NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT];

export const EVENT_LABELS: Record<string, { title: string; description: string }> = {
  [NOTIFICATION_EVENT_TASK_SESSION_WAITING_FOR_INPUT]: {
    title: 'A session has finished',
    description: 'Notify when an agent is waiting for your reply.',
  },
};
