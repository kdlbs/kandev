import type { RecoveryAuthMethod } from "@/components/task/chat/types";
import type { AgentErrorCause } from "@/lib/types/task-status-summary";

const MAX_METHODS = 8;
const MAX_METHOD_BYTES = 4096;
const MAX_METHOD_ID_BYTES = 128;
const MAX_METHOD_NAME_BYTES = 256;
const MAX_METHOD_DESCRIPTION_BYTES = 512;
const MAX_TERMINAL_WORD_BYTES = 256;
const MAX_TERMINAL_ARGS = 16;

type AuthenticationRecoveryMetadata = {
  is_auth_error?: unknown;
  auth_methods?: unknown;
} | null;

/** Resolve the durable cause first, with matching recovery metadata as legacy evidence. */
export function resolveAuthenticationRecovery(
  causes: readonly AgentErrorCause[] | undefined,
  metadata?: AuthenticationRecoveryMetadata,
): { required: boolean; methods: RecoveryAuthMethod[] } {
  const required = causes?.length
    ? causes.some((cause) => cause.code === "authentication_required")
    : metadata?.is_auth_error === true;
  if (!required) return { required: false, methods: [] };
  return { required: true, methods: normalizeAuthenticationMethods(metadata?.auth_methods) };
}

function normalizeAuthenticationMethods(value: unknown): RecoveryAuthMethod[] {
  if (!Array.isArray(value)) return [];
  const methods: RecoveryAuthMethod[] = [];
  for (const candidate of value.slice(0, MAX_METHODS * 4)) {
    if (methods.length >= MAX_METHODS || !isRecord(candidate)) continue;
    const id = safeText(candidate.id, MAX_METHOD_ID_BYTES);
    if (!id) continue;
    const name = safeText(candidate.name, MAX_METHOD_NAME_BYTES) || id;
    const method: RecoveryAuthMethod = {
      id,
      name,
      ...(safeText(candidate.description, MAX_METHOD_DESCRIPTION_BYTES)
        ? { description: safeText(candidate.description, MAX_METHOD_DESCRIPTION_BYTES) }
        : {}),
    };
    const terminalAuth = normalizeTerminalAuth(candidate.terminal_auth);
    if (terminalAuth) method.terminal_auth = terminalAuth;
    const projected = [...methods, method];
    if (serializedBytes(projected) > MAX_METHOD_BYTES) continue;
    methods.push(method);
  }
  return methods;
}

function normalizeTerminalAuth(value: unknown): RecoveryAuthMethod["terminal_auth"] {
  if (!isRecord(value)) return undefined;
  const rawArgs = value.args ?? [];
  if (!Array.isArray(rawArgs)) return undefined;
  const command = safeTerminalWord(value.command);
  const args = rawArgs.map((argument: unknown) => safeTerminalWord(argument));
  if (!command || args.length > MAX_TERMINAL_ARGS || args.some((arg) => !arg)) return undefined;
  const label = safeText(value.label, MAX_TERMINAL_WORD_BYTES);
  return {
    command,
    ...(args.length > 0 ? { args: args as string[] } : {}),
    ...(label ? { label } : {}),
  };
}

function safeTerminalWord(value: unknown): string | undefined {
  const word = safeText(value, MAX_TERMINAL_WORD_BYTES);
  return word && /^[A-Za-z0-9_./:@%+=,-]+$/.test(word) ? word : undefined;
}

function safeText(value: unknown, limit: number): string | undefined {
  if (typeof value !== "string") return undefined;
  const trimmed = value.trim();
  if (
    !trimmed ||
    new TextEncoder().encode(trimmed).length > limit ||
    /[\u0000-\u001f\u007f]/.test(trimmed)
  ) {
    return undefined;
  }
  return trimmed;
}

function serializedBytes(value: unknown): number {
  return new TextEncoder().encode(JSON.stringify(value)).length;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}
