import type {
  AgentUsageDTO,
  CompletedTaskActivityDTO,
  DailyActivityDTO,
  GitStatsDTO,
  GlobalStatsDTO,
  RepositoryStatsDTO,
  TaskStatsDTO,
} from "@/lib/types/http";
import { DEMO_IDS, demoRepository, type DemoState } from "./scenario";

export type DemoStatsSection =
  | "global"
  | "tasks"
  | "daily-activity"
  | "completed-activity"
  | "agent-usage"
  | "repositories"
  | "git";

const taskDurations = [1_440_000, 2_280_000, 720_000, 1_080_000, 1_860_000];

export function createDemoStats(section: string, state: DemoState): unknown | undefined {
  const taskStats = createTaskStats(state);
  const git: GitStatsDTO = {
    total_commits: 18,
    total_files_changed: 47,
    total_insertions: 1_284,
    total_deletions: 396,
  };

  switch (section as DemoStatsSection) {
    case "global":
      return createGlobalStats(state, taskStats);
    case "tasks":
      return { task_stats: taskStats, task_stats_has_more: false };
    case "daily-activity":
      return createDailyActivity();
    case "completed-activity":
      return createCompletedActivity();
    case "agent-usage":
      return createAgentUsage(state, taskStats);
    case "repositories":
      return createRepositoryStats(state, taskStats, git);
    case "git":
      return git;
    default:
      return undefined;
  }
}

function createTaskStats(state: DemoState): TaskStatsDTO[] {
  return state.tasks.map((task, index) => {
    const sessions = state.sessions.filter((session) => session.task_id === task.id);
    const messages = sessions.flatMap((session) => state.messagesBySession[session.id] ?? []);
    const duration = taskDurations[index % taskDurations.length];
    return {
      task_id: task.id,
      task_title: task.title,
      workspace_id: DEMO_IDS.workspace,
      workflow_id: DEMO_IDS.workflow,
      state: task.state,
      session_count: sessions.length,
      turn_count: Math.max(messages.filter((message) => message.author_type === "user").length, 1),
      message_count: messages.length,
      user_message_count: messages.filter((message) => message.author_type === "user").length,
      tool_call_count: messages.filter((message) => message.type !== "message").length,
      total_duration_ms: duration,
      active_duration_ms: Math.round(duration * 0.72),
      elapsed_span_ms: Math.round(duration * 1.15),
      created_at: task.created_at,
      completed_at: task.state === "COMPLETED" ? task.updated_at : undefined,
    };
  });
}

function createGlobalStats(state: DemoState, tasks: TaskStatsDTO[]): GlobalStatsDTO {
  const totalDuration = tasks.reduce((sum, task) => sum + task.total_duration_ms, 0);
  const turns = tasks.reduce((sum, task) => sum + task.turn_count, 0);
  const messages = tasks.reduce((sum, task) => sum + task.message_count, 0);
  return {
    total_tasks: state.tasks.length,
    completed_tasks: state.tasks.filter((task) => task.state === "COMPLETED").length,
    in_progress_tasks: state.tasks.filter((task) => task.state === "IN_PROGRESS").length,
    total_sessions: state.sessions.length,
    total_turns: turns,
    total_messages: messages,
    total_user_messages: tasks.reduce((sum, task) => sum + task.user_message_count, 0),
    total_tool_calls: tasks.reduce((sum, task) => sum + task.tool_call_count, 0),
    total_duration_ms: totalDuration,
    avg_turns_per_task: average(turns, state.tasks.length),
    avg_messages_per_task: average(messages, state.tasks.length),
    avg_duration_ms_per_task: average(totalDuration, state.tasks.length),
    avg_turn_duration_ms: average(totalDuration, turns),
    avg_messages_per_turn: average(messages, turns),
  };
}

function createDailyActivity(): DailyActivityDTO[] {
  return [
    { date: "2026-07-12", turn_count: 3, message_count: 14, task_count: 2 },
    { date: "2026-07-13", turn_count: 5, message_count: 22, task_count: 3 },
    { date: "2026-07-14", turn_count: 4, message_count: 18, task_count: 2 },
    { date: "2026-07-15", turn_count: 7, message_count: 31, task_count: 4 },
    { date: "2026-07-16", turn_count: 6, message_count: 27, task_count: 3 },
    { date: "2026-07-17", turn_count: 8, message_count: 36, task_count: 4 },
    { date: "2026-07-18", turn_count: 5, message_count: 24, task_count: 3 },
  ];
}

function createCompletedActivity(): CompletedTaskActivityDTO[] {
  return [
    { date: "2026-07-12", completed_tasks: 1 },
    { date: "2026-07-13", completed_tasks: 0 },
    { date: "2026-07-14", completed_tasks: 2 },
    { date: "2026-07-15", completed_tasks: 1 },
    { date: "2026-07-16", completed_tasks: 2 },
    { date: "2026-07-17", completed_tasks: 1 },
    { date: "2026-07-18", completed_tasks: 1 },
  ];
}

function createAgentUsage(state: DemoState, tasks: TaskStatsDTO[]): AgentUsageDTO[] {
  return [
    {
      agent_profile_id: DEMO_IDS.profile,
      agent_profile_name: "Mock agent",
      agent_model: "Browser demo model",
      session_count: state.sessions.length,
      turn_count: tasks.reduce((sum, task) => sum + task.turn_count, 0),
      total_duration_ms: tasks.reduce((sum, task) => sum + task.total_duration_ms, 0),
    },
  ];
}

function createRepositoryStats(
  state: DemoState,
  tasks: TaskStatsDTO[],
  git: GitStatsDTO,
): RepositoryStatsDTO[] {
  return [
    {
      repository_id: demoRepository.id,
      repository_name: demoRepository.name,
      total_tasks: state.tasks.length,
      completed_tasks: state.tasks.filter((task) => task.state === "COMPLETED").length,
      in_progress_tasks: state.tasks.filter((task) => task.state === "IN_PROGRESS").length,
      session_count: state.sessions.length,
      turn_count: tasks.reduce((sum, task) => sum + task.turn_count, 0),
      message_count: tasks.reduce((sum, task) => sum + task.message_count, 0),
      user_message_count: tasks.reduce((sum, task) => sum + task.user_message_count, 0),
      tool_call_count: tasks.reduce((sum, task) => sum + task.tool_call_count, 0),
      total_duration_ms: tasks.reduce((sum, task) => sum + task.total_duration_ms, 0),
      ...git,
    },
  ];
}

function average(total: number, count: number): number {
  return count === 0 ? 0 : Math.round((total / count) * 10) / 10;
}
