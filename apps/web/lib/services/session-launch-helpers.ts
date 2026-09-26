import type {
  LaunchActivationSource,
  LaunchSessionRequest,
  MessageAttachment,
} from "./session-launch-service";
import type { TaskPriority } from "@/lib/types/http";

export type LayoutIntentHint = "default" | "plan" | "pr-review" | "keep";

type BuildResult = {
  request: LaunchSessionRequest;
  layout: LayoutIntentHint;
};

export function buildStartRequest(
  taskId: string,
  agentProfileId: string,
  opts?: {
    executorId?: string;
    executorProfileId?: string;
    prompt?: string;
    planMode?: boolean;
    priority?: TaskPriority;
    autoStart?: boolean;
    profileExplicit?: boolean;
    attachments?: MessageAttachment[];
    autoCreatePR?: boolean;
  },
): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "start",
      agent_profile_id: agentProfileId,
      executor_id: opts?.executorId,
      executor_profile_id: opts?.executorProfileId,
      prompt: opts?.prompt,
      plan_mode: opts?.planMode,
      priority: opts?.priority,
      auto_start: opts?.autoStart,
      attachments: opts?.attachments,
      auto_create_pr: opts?.autoCreatePR || undefined,
      ...(opts?.profileExplicit !== undefined ? { profile_explicit: opts.profileExplicit } : {}),
    },
    layout: opts?.planMode ? "plan" : "default",
  };
}

export function buildPrepareRequest(
  taskId: string,
  opts?: { agentProfileId?: string; executorId?: string; executorProfileId?: string },
): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "prepare",
      agent_profile_id: opts?.agentProfileId,
      executor_id: opts?.executorId,
      executor_profile_id: opts?.executorProfileId,
      launch_workspace: true,
    },
    layout: "default",
  };
}

export function buildPRPrepareRequest(taskId: string): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "prepare",
      launch_workspace: true,
    },
    layout: "pr-review",
  };
}

export function buildStartCreatedRequest(
  taskId: string,
  sessionId: string,
  opts?: { agentProfileId?: string; prompt?: string; skipMessageRecord?: boolean },
): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "start_created",
      session_id: sessionId,
      agent_profile_id: opts?.agentProfileId,
      prompt: opts?.prompt,
      skip_message_record: opts?.skipMessageRecord,
    },
    layout: "keep",
  };
}

export function buildResumeRequest(
  taskId: string,
  sessionId: string,
  opts?: { activationSource?: LaunchActivationSource },
): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "resume",
      session_id: sessionId,
      ...(opts?.activationSource ? { activation_source: opts.activationSource } : {}),
    },
    layout: "keep",
  };
}

export function buildRestoreWorkspaceRequest(taskId: string, sessionId: string): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "restore_workspace",
      session_id: sessionId,
    },
    layout: "default",
  };
}

export function buildWorkflowStepRequest(
  taskId: string,
  sessionId: string,
  stepId: string,
): BuildResult {
  return {
    request: {
      task_id: taskId,
      intent: "workflow_step",
      session_id: sessionId,
      workflow_step_id: stepId,
    },
    layout: "keep",
  };
}
