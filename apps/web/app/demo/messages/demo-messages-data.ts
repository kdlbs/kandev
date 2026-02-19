import type { Message } from "@/lib/types/http";
import { MARKDOWN_DEMO_MESSAGE_CONTENT } from "./demo-messages-content";

const AUTH_CONFIG_PATH = "src/auth/config.ts";
const B = { session_id: "demo" as const, task_id: "demo-task" };

export const dummyMessages: Message[] = [
  // Task description
  {
    ...B,
    id: "task-description",
    content:
      "[TASK DESCRIPTION - AMBER BANNER] Implement a new feature to handle user authentication with OAuth 2.0",
    author_type: "user",
    type: "message",
    created_at: "2024-01-20T10:00:00Z",
  },

  // User message - short
  {
    ...B,
    id: "user-1",
    content: "[USER MESSAGE - SHORT] Can you help me with this?",
    author_type: "user",
    type: "message",
    created_at: "2024-01-20T10:01:00Z",
  },

  // User message - long
  {
    ...B,
    id: "user-2",
    content:
      "[USER MESSAGE - LONG] I need to implement a complex feature that involves multiple components. The requirements are:\n1. User authentication\n2. Role-based access control\n3. Session management\n4. Token refresh mechanism\n\nCan you help me design this system?",
    author_type: "user",
    type: "message",
    created_at: "2024-01-20T10:02:00Z",
  },

  // Agent message - plain text
  {
    ...B,
    id: "agent-1",
    content:
      "[AGENT MESSAGE - PLAIN TEXT] This is a simple paragraph with no markdown formatting. Just plain text content.",
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:03:00Z",
  },

  // Agent message - with markdown
  {
    id: "agent-2",
    ...B,
    content: MARKDOWN_DEMO_MESSAGE_CONTENT,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:04:00Z",
  },

  // Thinking message
  {
    id: "thinking-1",
    ...B,
    type: "thinking",
    content: "",
    author_type: "agent",
    created_at: "2024-01-20T10:05:00Z",
    metadata: {
      thinking:
        "[THINKING MESSAGE] This shows the agent's internal reasoning process. I need to check the current authentication implementation before suggesting changes. Let me search for existing auth files and understand the current architecture.",
    },
  },

  // Tool call - Read (complete)
  {
    id: "tool-1",
    ...B,
    type: "tool_call",
    content: "Read file",
    author_type: "agent",
    created_at: "2024-01-20T10:06:00Z",
    metadata: {
      tool_call_id: "tool-1",
      tool_name: "Read",
      title: "[TOOL - READ - COMPLETE \u2713] Read src/auth/config.ts",
      status: "complete",
      args: {
        file_path: "/Users/demo/project/src/auth/config.ts",
        line_count: 50,
      },
      result: "File content loaded successfully (50 lines)",
    },
  },

  // Tool call - Bash (running)
  {
    id: "tool-2",
    ...B,
    type: "tool_call",
    content: "Run command",
    author_type: "agent",
    created_at: "2024-01-20T10:07:00Z",
    metadata: {
      tool_call_id: "tool-2",
      tool_name: "Bash",
      title: "[TOOL - BASH - RUNNING \u231B] npm test",
      status: "running",
      args: {
        command: "npm test -- --coverage",
        description: "Run tests with coverage",
        timeout: 30000,
      },
    },
  },

  // Tool call - Edit (error)
  {
    id: "tool-3",
    ...B,
    type: "tool_call",
    content: "Edit file",
    author_type: "agent",
    created_at: "2024-01-20T10:08:00Z",
    metadata: {
      tool_call_id: "tool-3",
      tool_name: "Edit",
      title: "[TOOL - EDIT - ERROR \u2717] Edit src/components/Login.tsx",
      status: "error",
      args: {
        file_path: "/Users/demo/project/src/components/Login.tsx",
        old_string: "const handleLogin = () => {",
        new_string: "const handleLogin = async () => {",
      },
      result:
        "Error: old_string not found in file. Make sure the old_string exactly matches the content in the file.",
    },
  },

  // Tool call - Edit with complex metadata (complete)
  {
    id: "tool-3b",
    ...B,
    type: "tool_call",
    content: "Auggie Read `drizzle.config.ts`",
    author_type: "agent",
    created_at: "2024-01-20T10:08:30Z",
    metadata: {
      tool_call_id: "toolu_vrtx_01W75yrLi1WLgLCz8P3D8jjh",
      tool_name: "edit",
      title: "Edit drizzle.config.ts",
      status: "complete",
      args: {
        kind: "edit",
        locations: [
          {
            path: "/Users/cfl/.kandev/worktrees/1_c28de20d/drizzle.config.ts",
          },
        ],
        path: "/Users/cfl/.kandev/worktrees/1_c28de20d/drizzle.config.ts",
        raw_input: {
          command: "str_replace",
          instruction_reminder:
            "ALWAYS BREAK DOWN EDITS INTO SMALLER CHUNKS OF AT MOST 150 LINES EACH.",
          new_str_1:
            'export default defineConfig({\n  schema: "./src/main/lib/db/schema/index.ts",\n  dialect: "sqlite",\n})',
          old_str_1:
            'export default defineConfig({\n  schema: "./src/main/lib/db/schema/index.ts",\n  out: "./drizzle",\n  dialect: "sqlite",\n})',
          old_str_end_line_number_1: 7,
          old_str_start_line_number_1: 3,
          path: "drizzle.config.ts",
        },
      },
    },
  },

  // Tool call - Search (pending, with permission)
  {
    id: "tool-4",
    ...B,
    type: "tool_call",
    content: "Search files",
    author_type: "agent",
    created_at: "2024-01-20T10:09:00Z",
    metadata: {
      tool_call_id: "tool-4",
      tool_name: "Grep",
      title: '[TOOL - GREP - PENDING \uD83D\uDD12] Search for "authentication" in codebase',
      status: "pending",
      args: {
        pattern: "authentication",
        path: "/Users/demo/project/src",
        output_mode: "files_with_matches",
      },
    },
  },

  // Permission request (for tool-4)
  {
    id: "permission-1",
    ...B,
    type: "permission_request",
    content: "[PERMISSION REQUEST] Approve/Reject action",
    author_type: "user",
    created_at: "2024-01-20T10:09:01Z",
    metadata: {
      pending_id: "perm-1",
      tool_call_id: "tool-4",
      action_type: "file_search",
      action_details: {
        path: "/Users/demo/project/src",
        command: 'grep -r "authentication"',
      },
      options: [
        { option_id: "allow-1", name: "Allow Once", kind: "allow_once" },
        { option_id: "allow-2", name: "Allow Always", kind: "allow_always" },
        { option_id: "reject-1", name: "Reject", kind: "reject_once" },
      ],
      status: "pending",
    },
  },

  // Status message - simple (separator style)
  {
    id: "status-0",
    ...B,
    type: "status",
    content: "Agent initialized",
    author_type: "agent",
    created_at: "2024-01-20T10:09:30Z",
  },

  // Status message - success
  {
    id: "status-1",
    ...B,
    type: "status",
    content: "[STATUS MESSAGE - SUCCESS \u2713] Tests passed successfully! All 42 tests completed.",
    author_type: "agent",
    created_at: "2024-01-20T10:10:00Z",
    metadata: {
      level: "success",
    },
  },

  // Status message - error
  {
    id: "status-2",
    ...B,
    type: "error",
    content:
      "[STATUS MESSAGE - ERROR \u2717] Build failed: TypeScript compilation error in auth.ts:45",
    author_type: "agent",
    created_at: "2024-01-20T10:11:00Z",
    metadata: {
      level: "error",
    },
  },

  // Todo message
  {
    id: "todo-1",
    ...B,
    type: "todo",
    content: "[TODO MESSAGE] Task list",
    author_type: "agent",
    created_at: "2024-01-20T10:12:00Z",
    metadata: {
      todos: [
        { id: "1", text: "[TODO ITEM - COMPLETED] Set up OAuth provider", completed: true },
        {
          id: "2",
          text: "[TODO ITEM - PENDING] Implement token validation middleware",
          completed: false,
        },
        { id: "3", text: "[TODO ITEM - PENDING] Add refresh token endpoint", completed: false },
        { id: "4", text: "[TODO ITEM - PENDING] Write integration tests", completed: false },
      ],
    },
  },

  // Script execution message - running
  {
    id: "script-1",
    ...B,
    type: "script_execution",
    content:
      "[SCRIPT EXECUTION MESSAGE - RUNNING] Setting up environment...\nInstalling dependencies...",
    author_type: "agent",
    created_at: "2024-01-20T10:13:00Z",
    metadata: {
      script_type: "setup",
      command: "./scripts/setup-env.sh",
      status: "running",
    },
  },

  // Script execution message - completed
  {
    id: "script-2",
    ...B,
    type: "script_execution",
    content:
      "[SCRIPT EXECUTION MESSAGE - COMPLETED] Build successful!\nAll tests passed.\nDeployment ready.",
    author_type: "agent",
    created_at: "2024-01-20T10:14:00Z",
    metadata: {
      script_type: "cleanup",
      command: "npm run build",
      status: "exited",
      exit_code: 0,
      started_at: "2024-01-20T10:13:00Z",
      completed_at: "2024-01-20T10:14:00Z",
    },
  },

  // Agent message - mixed content
  {
    id: "agent-3",
    ...B,
    content: `**[MIXED CONTENT DEMO]** Testing nested and mixed elements:

#### Heading 4 (h4 element)

Regular paragraph with inline elements: \`code\`, **bold**, and more text.

**Nested list structure:**

1. **First level item** (ol > li with strong)
   - Nested bullet (this should indent)
   - Another nested item
2. Second level item with \`inline code\`
3. Third item

**Technical details:**

- File path: \`/src/components/Auth.tsx\`
- Function: \`authenticateUser(credentials)\`
- Status: **Active** (strong element)

\`\`\`bash
# Code block with bash syntax
npm install
npm run dev
\`\`\`

Final paragraph (p element) to wrap up.`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:14:00Z",
  },

  // Agent message with rich blocks - thinking
  {
    id: "agent-rich-1",
    ...B,
    content: `**[RICH BLOCK - THINKING]** Here's my analysis of the authentication system.`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:15:00Z",
    metadata: {
      thinking:
        "I need to carefully consider the security implications here. The current implementation uses JWT tokens, but we should add refresh token rotation to prevent token theft. We also need to implement proper CSRF protection for the authentication endpoints.",
    },
  },

  // Agent message with rich blocks - todos
  {
    id: "agent-rich-2",
    ...B,
    content: `**[RICH BLOCK - TODOS]** Here are the next steps we need to take.`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:16:00Z",
    metadata: {
      todos: [
        { text: "Update the authentication middleware", done: true },
        { text: "Add unit tests for token validation", done: false },
        { text: "Implement rate limiting on login endpoint", done: false },
        "Update documentation for new OAuth flow",
      ],
    },
  },

  // Agent message with rich blocks - diff payload (structured)
  {
    id: "agent-rich-3",
    ...B,
    content: `**[RICH BLOCK - DIFF PAYLOAD]** I've made the following changes to the authentication config:`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:17:00Z",
    metadata: {
      diff: {
        oldFile: { fileName: AUTH_CONFIG_PATH, fileLang: "typescript" },
        newFile: { fileName: AUTH_CONFIG_PATH, fileLang: "typescript" },
        hunks: [
          `diff --git a/src/auth/config.ts b/src/auth/config.ts
index 1234567..abcdefg 100644
--- a/src/auth/config.ts
+++ b/src/auth/config.ts
@@ -1,7 +1,10 @@
 export const authConfig = {
   clientId: process.env.OAUTH_CLIENT_ID,
-  clientSecret: process.env.OAUTH_CLIENT_SECRET,
+  clientSecret: process.env.OAUTH_CLIENT_SECRET || '',
   redirectUri: process.env.OAUTH_REDIRECT_URI,
+  tokenExpiry: '1h',
+  refreshTokenExpiry: '7d',
+  enableCsrf: true,
 };

 export function validateToken(token: string) {`,
        ],
      },
    },
  },

  // Agent message with rich blocks - diff string (plain text)
  {
    id: "agent-rich-4",
    ...B,
    content: `**[RICH BLOCK - DIFF STRING]** Here's the diff for the middleware changes:`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:18:00Z",
    metadata: {
      diff: `--- a/src/middleware/auth.ts
+++ b/src/middleware/auth.ts
@@ -12,6 +12,11 @@ export async function authMiddleware(req: Request, res: Response, next: NextFun
   const token = req.headers.authorization?.split(' ')[1];

   if (!token) {
-    return res.status(401).json({ error: 'No token provided' });
+    return res.status(401).json({
+      error: 'No token provided',
+      code: 'AUTH_TOKEN_MISSING',
+      timestamp: new Date().toISOString()
+    });
   }

   try {`,
    },
  },

  // Agent message with multiple rich blocks
  {
    id: "agent-rich-5",
    ...B,
    content: `**[RICH BLOCK - ALL TYPES]** Here's my complete analysis with all the changes:`,
    author_type: "agent",
    type: "message",
    created_at: "2024-01-20T10:19:00Z",
    metadata: {
      thinking:
        "After reviewing the code, I found several areas that need improvement. The error handling is inconsistent and we need better type safety.",
      todos: [
        { text: "Add TypeScript strict mode", done: true },
        { text: "Implement error boundaries", done: false },
        { text: "Add logging middleware", done: false },
      ],
      diff: {
        oldFile: { fileName: "src/types/auth.ts", fileLang: "typescript" },
        newFile: { fileName: "src/types/auth.ts", fileLang: "typescript" },
        hunks: [
          `diff --git a/src/types/auth.ts b/src/types/auth.ts
index 9876543..fedcba9 100644
--- a/src/types/auth.ts
+++ b/src/types/auth.ts
@@ -1,4 +1,12 @@
-export type AuthToken = string;
+export interface AuthToken {
+  value: string;
+  expiresAt: Date;
+  type: 'access' | 'refresh';
+}

-export type User = {
+export interface User {
   id: string;
   email: string;
+  role: 'admin' | 'user';
+  createdAt: Date;
+}`,
        ],
      },
    },
  },

  // === Normalized Tool Messages (tool_edit, tool_read, tool_execute) ===

  // Tool Edit - complete with diff
  {
    id: "tool-edit-1",
    ...B,
    type: "tool_edit",
    content: "Edit src/auth/config.ts",
    author_type: "agent",
    created_at: "2024-01-20T10:20:00Z",
    metadata: {
      tool_call_id: "tool-edit-1",
      status: "complete",
      file_path: AUTH_CONFIG_PATH,
      old_content: "  clientSecret: process.env.OAUTH_CLIENT_SECRET,",
      new_content: "  clientSecret: process.env.OAUTH_CLIENT_SECRET || '',\n  tokenExpiry: '1h',",
      diff: {
        hunks: [
          `diff --git a/src/auth/config.ts b/src/auth/config.ts
index 1234567..abcdefg 100644
--- a/src/auth/config.ts
+++ b/src/auth/config.ts
@@ -2,3 +2,4 @@
   clientId: process.env.OAUTH_CLIENT_ID,
-  clientSecret: process.env.OAUTH_CLIENT_SECRET,
+  clientSecret: process.env.OAUTH_CLIENT_SECRET || '',
+  tokenExpiry: '1h',
   redirectUri: process.env.OAUTH_REDIRECT_URI,`,
        ],
        oldFile: { fileName: AUTH_CONFIG_PATH, fileLang: "typescript" },
        newFile: { fileName: AUTH_CONFIG_PATH, fileLang: "typescript" },
      },
      result: "File edited successfully.",
    },
  },

  // Tool Edit - running (expanded)
  {
    id: "tool-edit-2",
    ...B,
    type: "tool_edit",
    content: "Edit src/middleware/auth.ts",
    author_type: "agent",
    created_at: "2024-01-20T10:20:30Z",
    metadata: {
      tool_call_id: "tool-edit-2",
      status: "running",
      file_path: "src/middleware/auth.ts",
      old_content: "return res.status(401).json({ error: 'No token' });",
      new_content:
        "return res.status(401).json({\n  error: 'No token',\n  code: 'AUTH_MISSING'\n});",
      diff: {
        hunks: [
          `diff --git a/src/middleware/auth.ts b/src/middleware/auth.ts
--- a/src/middleware/auth.ts
+++ b/src/middleware/auth.ts
@@ -12,1 +12,4 @@
-return res.status(401).json({ error: 'No token' });
+return res.status(401).json({
+  error: 'No token',
+  code: 'AUTH_MISSING'
+});`,
        ],
        oldFile: { fileName: "src/middleware/auth.ts", fileLang: "typescript" },
        newFile: { fileName: "src/middleware/auth.ts", fileLang: "typescript" },
      },
    },
  },

  // Tool Edit - error
  {
    id: "tool-edit-3",
    ...B,
    type: "tool_edit",
    content: "Edit src/utils/format.ts",
    author_type: "agent",
    created_at: "2024-01-20T10:21:00Z",
    metadata: {
      tool_call_id: "tool-edit-3",
      status: "error",
      file_path: "src/utils/format.ts",
      result: "Error: old_string not found in file.",
    },
  },

  // Tool Read - complete
  {
    id: "tool-read-1",
    ...B,
    type: "tool_read",
    content: "Read src/auth/config.ts",
    author_type: "agent",
    created_at: "2024-01-20T10:22:00Z",
    metadata: {
      tool_call_id: "tool-read-1",
      status: "complete",
      file_path: AUTH_CONFIG_PATH,
      line_count: 50,
    },
  },

  // Tool Read - running
  {
    id: "tool-read-2",
    ...B,
    type: "tool_read",
    content: "Read package.json",
    author_type: "agent",
    created_at: "2024-01-20T10:22:30Z",
    metadata: {
      tool_call_id: "tool-read-2",
      status: "running",
      file_path: "package.json",
    },
  },

  // Tool Read - error
  {
    id: "tool-read-3",
    ...B,
    type: "tool_read",
    content: "Read /nonexistent/file.ts",
    author_type: "agent",
    created_at: "2024-01-20T10:22:45Z",
    metadata: {
      tool_call_id: "tool-read-3",
      status: "error",
      file_path: "/nonexistent/file.ts",
    },
  },

  // Tool Execute - complete (success)
  {
    id: "tool-exec-1",
    ...B,
    type: "tool_execute",
    content: "Run `npm test -- --coverage`",
    author_type: "agent",
    created_at: "2024-01-20T10:23:00Z",
    metadata: {
      tool_call_id: "tool-exec-1",
      status: "complete",
      command: "npm test -- --coverage",
      cwd: "/workspace",
      exit_code: 0,
      stdout:
        "PASS  src/auth/config.test.ts\nPASS  src/middleware/auth.test.ts\n\nTest Suites:  2 passed, 2 total\nTests:        12 passed, 12 total\nCoverage:     87.5%",
      result: "Tests passed successfully.",
    },
  },

  // Tool Execute - running
  {
    id: "tool-exec-2",
    ...B,
    type: "tool_execute",
    content: "Run `go build ./...`",
    author_type: "agent",
    created_at: "2024-01-20T10:23:30Z",
    metadata: {
      tool_call_id: "tool-exec-2",
      status: "running",
      command: "go build ./...",
      cwd: "/workspace",
    },
  },

  // Tool Execute - error with stderr
  {
    id: "tool-exec-3",
    ...B,
    type: "tool_execute",
    content: "Run `npm run lint`",
    author_type: "agent",
    created_at: "2024-01-20T10:24:00Z",
    metadata: {
      tool_call_id: "tool-exec-3",
      status: "complete",
      command: "npm run lint",
      exit_code: 1,
      stdout: "",
      stderr:
        "Error: src/auth/config.ts(5,1): Missing semicolon.\nError: src/middleware/auth.ts(12,3): Unexpected token.",
      result: "Lint failed with 2 errors.",
    },
  },
];
