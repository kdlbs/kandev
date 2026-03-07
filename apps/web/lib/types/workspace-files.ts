export type FileTreeNode = {
  name: string;
  path: string;
  is_dir: boolean;
  size?: number;
  children?: FileTreeNode[];
};

export type FileTreeResponse = {
  request_id?: string;
  root: FileTreeNode;
  error?: string;
};

export type FileContentResponse = {
  request_id?: string;
  path: string;
  content: string;
  size: number;
  is_binary?: boolean;
  resolved_path?: string;
  error?: string;
};

export type FileSearchResponse = {
  files: string[];
  error?: string;
};

export type FileChangeEvent = {
  timestamp: string;
  path: string;
  operation: "create" | "write" | "remove" | "rename" | "chmod" | "refresh";
  session_id: string;
  task_id: string;
  agent_id: string;
};

export type FileChangeNotificationPayload = {
  session_id: string;
  changes: FileChangeEvent[];
};

export type OpenFileTab = {
  path: string;
  name: string;
  content: string;
  originalContent: string;
  originalHash: string;
  isDirty: boolean;
  isBinary?: boolean;
};

export const FILE_EXTENSION_COLORS: Record<string, string> = {
  ts: "bg-blue-500",
  tsx: "bg-blue-400",
  js: "bg-yellow-500",
  jsx: "bg-yellow-400",
  go: "bg-cyan-500",
  py: "bg-green-500",
  rs: "bg-orange-500",
  json: "bg-amber-400",
  css: "bg-purple-500",
  html: "bg-red-500",
  md: "bg-gray-400",
};
