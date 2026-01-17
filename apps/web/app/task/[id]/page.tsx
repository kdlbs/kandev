import { fetchTask, listTaskSessions } from '@/lib/http';
import { redirect } from 'next/navigation';
import { linkToTaskSession } from '@/lib/links';

export default async function TaskPage({ params }: { params: Promise<{ id: string }> }) {
  try {
    const { id } = await params;
    const task = await fetchTask(id, { cache: 'no-store' });
    const sessions = await listTaskSessions(id, { cache: 'no-store' });
    if (sessions.sessions.length > 0) {
      redirect(linkToTaskSession(id, sessions.sessions[0].id));
    }
    redirect(`/?boardId=${task.board_id}`);
  } catch {
    redirect('/');
  }
  return null;
}
