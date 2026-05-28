/**
 * Query options for the comments domain.
 *
 * IMPORTANT — No server fetch:
 * Comments in this domain (diff, plan, file-editor, PR-feedback) are
 * client-side annotations created by the user, persisted to sessionStorage,
 * and sent to the agent via `runComment`. They are NOT fetched from a server
 * endpoint, so there are no queryFn-backed queryOptions here.
 *
 * The Office domain has a separate server-backed task-comment concept
 * (listComments / createComment in office-extended-api.ts) that belongs
 * under qk.office.* and will be migrated as part of the Office wave.
 *
 * This file exists to satisfy the Wave 1 deliverable contract and to reserve
 * the namespace for potential future server persistence of user annotations.
 */

// No exports — intentional. Comments have no server state to query.
// The slice (lib/state/slices/comments/) and its hooks remain Zustand-only.
