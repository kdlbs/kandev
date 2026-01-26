'use client';

import type { Message } from '@/lib/types/http';
import { MessageRenderer } from '@/components/task/chat/message-renderer';

const dummyMessages: Message[] = [
  // Task description
  {
    id: 'task-description',
    session_id: 'demo',
    task_id: 'demo-task',
    content: '[TASK DESCRIPTION - AMBER BANNER] Implement a new feature to handle user authentication with OAuth 2.0',
    author_type: 'user',
    type: 'message',
    created_at: '2024-01-20T10:00:00Z',
  },

  // User message - short
  {
    id: 'user-1',
    session_id: 'demo',
    task_id: 'demo-task',
    content: '[USER MESSAGE - SHORT] Can you help me with this?',
    author_type: 'user',
    type: 'message',
    created_at: '2024-01-20T10:01:00Z',
  },

  // User message - long
  {
    id: 'user-2',
    session_id: 'demo',
    task_id: 'demo-task',
    content: '[USER MESSAGE - LONG] I need to implement a complex feature that involves multiple components. The requirements are:\n1. User authentication\n2. Role-based access control\n3. Session management\n4. Token refresh mechanism\n\nCan you help me design this system?',
    author_type: 'user',
    type: 'message',
    created_at: '2024-01-20T10:02:00Z',
  },

  // Agent message - plain text
  {
    id: 'agent-1',
    session_id: 'demo',
    task_id: 'demo-task',
    content: '[AGENT MESSAGE - PLAIN TEXT] This is a simple paragraph with no markdown formatting. Just plain text content.',
    author_type: 'agent',
    type: 'message',
    created_at: '2024-01-20T10:03:00Z',
  },

  // Agent message - with markdown
  {
    id: 'agent-2',
    session_id: 'demo',
    task_id: 'demo-task',
    content: `**[MARKDOWN DEMO]** Here's a message showing all markdown elements:

## Heading 2 (h2 element)

This is a **paragraph (p element)** with some **bold text (strong element)** to test styling.

### Heading 3 (h3 element)

**Ordered List (ol element):**

1. First item (li element in ol)
2. Second item with **bold text**
3. Third item with inline code: \`const example = true\`
4. Fourth item (li element)

**Unordered List (ul element):**

- Bullet point one (li element in ul)
- Bullet point two
- Bullet point with \`inline code\` (code element)
- Another bullet point

**Inline code examples (code element, inline):**
- Variable: \`const token = jwt.sign(payload)\`
- Function call: \`getUserData(userId)\`
- Path: \`/src/auth/config.ts\`

**Code block example (code element, block):**

\`\`\`typescript
// This is a code block (pre > code)
interface AuthConfig {
  clientId: string;
  clientSecret: string;
  redirectUri: string;
}

function authenticate(config: AuthConfig) {
  return jwt.sign(config);
}
\`\`\`

**Another code block (JavaScript):**

\`\`\`javascript
// Testing syntax highlighting
const data = { name: "test", value: 123 };
console.log(data);
\`\`\``,
    author_type: 'agent',
    type: 'message',
    created_at: '2024-01-20T10:04:00Z',
  },

  // Thinking message
  {
    id: 'thinking-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'thinking',
    content: '',
    author_type: 'agent',
    created_at: '2024-01-20T10:05:00Z',
    metadata: {
      thinking: '[THINKING MESSAGE] This shows the agent\'s internal reasoning process. I need to check the current authentication implementation before suggesting changes. Let me search for existing auth files and understand the current architecture.',
    },
  },

  // Tool call - Read (complete)
  {
    id: 'tool-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'tool_call',
    content: 'Read file',
    author_type: 'agent',
    created_at: '2024-01-20T10:06:00Z',
    metadata: {
      tool_call_id: 'tool-1',
      tool_name: 'Read',
      title: '[TOOL - READ - COMPLETE ✓] Read src/auth/config.ts',
      status: 'complete',
      args: {
        file_path: '/Users/demo/project/src/auth/config.ts',
        line_count: 50,
      },
      result: 'File content loaded successfully (50 lines)',
    },
  },

  // Tool call - Bash (running)
  {
    id: 'tool-2',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'tool_call',
    content: 'Run command',
    author_type: 'agent',
    created_at: '2024-01-20T10:07:00Z',
    metadata: {
      tool_call_id: 'tool-2',
      tool_name: 'Bash',
      title: '[TOOL - BASH - RUNNING ⏳] npm test',
      status: 'running',
      args: {
        command: 'npm test -- --coverage',
        description: 'Run tests with coverage',
        timeout: 30000,
      },
    },
  },

  // Tool call - Edit (error)
  {
    id: 'tool-3',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'tool_call',
    content: 'Edit file',
    author_type: 'agent',
    created_at: '2024-01-20T10:08:00Z',
    metadata: {
      tool_call_id: 'tool-3',
      tool_name: 'Edit',
      title: '[TOOL - EDIT - ERROR ✗] Edit src/components/Login.tsx',
      status: 'error',
      args: {
        file_path: '/Users/demo/project/src/components/Login.tsx',
        old_string: 'const handleLogin = () => {',
        new_string: 'const handleLogin = async () => {',
      },
      result: 'Error: old_string not found in file. Make sure the old_string exactly matches the content in the file.',
    },
  },

  // Tool call - Search (pending, with permission)
  {
    id: 'tool-4',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'tool_call',
    content: 'Search files',
    author_type: 'agent',
    created_at: '2024-01-20T10:09:00Z',
    metadata: {
      tool_call_id: 'tool-4',
      tool_name: 'Grep',
      title: '[TOOL - GREP - PENDING 🔒] Search for "authentication" in codebase',
      status: 'pending',
      args: {
        pattern: 'authentication',
        path: '/Users/demo/project/src',
        output_mode: 'files_with_matches',
      },
    },
  },

  // Permission request (for tool-4)
  {
    id: 'permission-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'permission_request',
    content: '[PERMISSION REQUEST] Approve/Reject action',
    author_type: 'user',
    created_at: '2024-01-20T10:09:01Z',
    metadata: {
      pending_id: 'perm-1',
      tool_call_id: 'tool-4',
      action_type: 'file_search',
      action_details: {
        path: '/Users/demo/project/src',
        command: 'grep -r "authentication"',
      },
      options: [
        { option_id: 'allow-1', name: 'Allow Once', kind: 'allow_once' },
        { option_id: 'allow-2', name: 'Allow Always', kind: 'allow_always' },
        { option_id: 'reject-1', name: 'Reject', kind: 'reject_once' },
      ],
      status: 'pending',
    },
  },

  // Status message - simple (separator style)
  {
    id: 'status-0',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'status',
    content: 'Agent initialized',
    author_type: 'agent',
    created_at: '2024-01-20T10:09:30Z',
  },

  // Status message - success
  {
    id: 'status-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'status',
    content: '[STATUS MESSAGE - SUCCESS ✓] Tests passed successfully! All 42 tests completed.',
    author_type: 'agent',
    created_at: '2024-01-20T10:10:00Z',
    metadata: {
      level: 'success',
    },
  },

  // Status message - error
  {
    id: 'status-2',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'error',
    content: '[STATUS MESSAGE - ERROR ✗] Build failed: TypeScript compilation error in auth.ts:45',
    author_type: 'agent',
    created_at: '2024-01-20T10:11:00Z',
    metadata: {
      level: 'error',
    },
  },

  // Todo message
  {
    id: 'todo-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'todo',
    content: '[TODO MESSAGE] Task list',
    author_type: 'agent',
    created_at: '2024-01-20T10:12:00Z',
    metadata: {
      todos: [
        { id: '1', text: '[TODO ITEM - COMPLETED] Set up OAuth provider', completed: true },
        { id: '2', text: '[TODO ITEM - PENDING] Implement token validation middleware', completed: false },
        { id: '3', text: '[TODO ITEM - PENDING] Add refresh token endpoint', completed: false },
        { id: '4', text: '[TODO ITEM - PENDING] Write integration tests', completed: false },
      ],
    },
  },

  // Script execution message - running
  {
    id: 'script-1',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'script_execution',
    content: '[SCRIPT EXECUTION MESSAGE - RUNNING] Setting up environment...\nInstalling dependencies...',
    author_type: 'agent',
    created_at: '2024-01-20T10:13:00Z',
    metadata: {
      script_type: 'setup',
      command: './scripts/setup-env.sh',
      status: 'running',
    },
  },

  // Script execution message - completed
  {
    id: 'script-2',
    session_id: 'demo',
    task_id: 'demo-task',
    type: 'script_execution',
    content: '[SCRIPT EXECUTION MESSAGE - COMPLETED] Build successful!\nAll tests passed.\nDeployment ready.',
    author_type: 'agent',
    created_at: '2024-01-20T10:14:00Z',
    metadata: {
      script_type: 'cleanup',
      command: 'npm run build',
      status: 'exited',
      exit_code: 0,
      started_at: '2024-01-20T10:13:00Z',
      completed_at: '2024-01-20T10:14:00Z',
    },
  },

  // Agent message - mixed content
  {
    id: 'agent-3',
    session_id: 'demo',
    task_id: 'demo-task',
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
    author_type: 'agent',
    type: 'message',
    created_at: '2024-01-20T10:14:00Z',
  },
];

export default function MessagesDemoPage() {
  const permissionsByToolCallId = new Map<string, Message>();
  const permMsg = dummyMessages.find(m => m.id === 'permission-1');
  if (permMsg) {
    permissionsByToolCallId.set('tool-4', permMsg);
  }

  return (
    <div className="min-h-screen bg-background p-8">
      <div className="max-w-4xl mx-auto">
        <div className="mb-8">
          <h1 className="text-2xl font-bold mb-2">Message Types Demo</h1>
          <p className="text-muted-foreground">
            All message types rendered for styling and testing
          </p>
        </div>

        <div className="space-y-2 border rounded-lg p-5 bg-card">
          {dummyMessages.map((message) => (
            <MessageRenderer
              key={message.id}
              comment={message}
              isTaskDescription={message.id === 'task-description'}
              taskId="demo-task"
              permissionsByToolCallId={permissionsByToolCallId}
            />
          ))}
        </div>
      </div>
    </div>
  );
}
