"use client";

// SSH executor settings UI. The two cards live in their own files so each
// stays under the component size limits; import from here to keep call sites
// short.
export { SSHConnectionCard } from "./ssh-connection-card";
export type { SSHConnectionCardProps, SSHExecutorConfig } from "./ssh-connection-card";
export { SSHSessionsCard } from "./ssh-sessions-card";
export type { SSHSessionsCardProps } from "./ssh-sessions-card";
