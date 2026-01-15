# Task Chat Interface - Final Implementation Plan

Comprehensive plan to build a high-performance, feature-rich task chat interface with SSR, lazy loading, and command support.

**Status:** Updated after rebase from main (Jan 2026)

**Key Changes:**
- ✅ SSR already implemented - fetches last 10 comments on server
- ✅ TanStack Virtual v3.13.18 already integrated
- ✅ Basic tool call rendering already exists
- ✅ Pagination API implemented (limit/before/after/sort + has_more/cursor)
- ✅ Smart auto-scroll added (only scrolls when at bottom)
- ✅ Lazy loading added (scroll-to-top fetch for older messages)
- ✅ CodeMirror input implemented (markdown, line wrapping, shortcut adapter, Mod-Enter submit)
- ✅ Slash + mention autocomplete added (dummy command examples)
- ✅ Message rendering refactor with adapter-style components
- ✅ Inline diff rendering (metadata diff uses @git-diff-view/react when structured)
- ⚠️ Rich content rendering partially implemented (thinking/todos/diff metadata cards, status/error/progress)
- ❌ Rich content (thinking, todos, diffs) not fully implemented

---

## Architecture Overview

### Data Flow

```
1. Initial Load (SSR):
   Server Component → HTTP GET /api/tasks/:id/comments?limit=10&sort=desc
   → Hydrate store with last 10 messages
   → Client renders, scrolled to bottom

2. WebSocket Subscription:
   Client mounts → ws.subscribe(taskId)
   → Receives comment.added notifications
   → Appends new messages to store
   → Auto-scroll if at bottom

3. Lazy Loading (Scroll Up):
   User scrolls to top → Detect via TanStack Virtual
   → If not all messages loaded: HTTP GET /api/tasks/:id/comments?before=:oldestId&limit=20
   → Prepend to store
   → Maintain scroll position
```

### Tech Stack

**Core:**
- React 18 + Next.js 14 (SSR)
- TanStack Virtual v3 (virtualization)
- Zustand + Immer (state)
- WebSocket (real-time)

**UI:**
- Tailwind CSS + shadcn/ui
- CodeMirror 6 (input + file editing)
- react-markdown + remark-gfm (display)
- @git-diff-view/react (existing)

**New Dependencies:**
```json
{
  "@codemirror/view": "^6.34.1",
  "@codemirror/state": "^6.4.1",
  "@codemirror/lang-javascript": "^6.2.2",
  "@codemirror/lang-markdown": "^6.3.1",
  "@codemirror/autocomplete": "^6.18.1",
  "@codemirror/commands": "^6.7.0",
  "@codemirror/language": "^6.10.3",
  "@codemirror/theme-one-dark": "^6.1.2",
  "codemirror": "^6.0.1"
}
```

**Total:** ~150KB gzipped (unified solution for chat + file editing)

**Already Installed:**
- `@tanstack/react-virtual`: v3.13.18 ✅
- `react-markdown` + `remark-gfm` ✅
- `@git-diff-view/react` ✅

---

## Current Implementation Status

### ✅ Already Implemented (After Rebase)

1. **SSR (Server-Side Rendering)**
   - `app/task/[id]/page.tsx` fetches comments via `listTaskComments()`
   - State hydration via `StateHydrator`
   - Fetches last 10 comments on initial load (sorted asc for display)

2. **TanStack Virtual**
   - Version: 3.13.18
   - Location: `task-chat-panel.tsx` (line 256)
   - Current config: `estimateSize: 96`, `overscan: 6`
   - Auto-scroll to bottom on new messages

3. **HTTP API Endpoint**
   - Backend: `GET /api/v1/tasks/:id/comments` (limit/before/after/sort)
   - Frontend: `listTaskComments(taskId, params)` in `lib/http/index.ts`
   - Response: `{ comments, total, has_more, cursor }`

4. **Basic Tool Call Rendering**
   - Expandable tool call cards
   - Tool icons and status indicators
   - Args/result display

### ❌ Still Missing (Needs Implementation)

1. **Rich Content Display**
   - Basic thinking/todos/diff blocks rendered when metadata exists
   - Error/status/progress cards added
   - Inline diff viewer now supported for structured metadata
   - Remaining: tool call enhancements, error card details, thinking toggle, diff extraction from tool results

4. **CodeMirror Input**
   - Still using basic Textarea
   - No markdown mode or syntax highlighting
   - No command/mention autocomplete

5. **Rich Content Display**
   - No thinking message display
   - No todo list visualization
   - No inline diff viewer
   - Basic error display only

---

## Implementation Phases

### Phase 1: Pagination API + Lazy Loading (Week 1)

#### 1.1 Backend API Enhancements

**Update Existing Endpoint:**
```go
// File: apps/backend/internal/task/dto/requests.go
type ListCommentsRequest struct {
  TaskID string
  Limit  int    // default 10, max 100
  Before string // comment ID for pagination (older)
  After  string // comment ID for newer messages
  Sort   string // "asc" | "desc", default "desc"
}

// File: apps/backend/internal/task/dto/dto.go
type ListCommentsResponse struct {
  Comments []*v1.Comment `json:"comments"`
  Total    int           `json:"total"`
  HasMore  bool          `json:"has_more"`   // NEW
  Cursor   string        `json:"cursor"`      // NEW
}
```

**Implementation Steps:**
1. Update `ListCommentsRequest` DTO to accept query params
2. Modify `CommentController.ListComments()` to pass pagination params
3. Update `Service.ListComments()` to support cursor-based queries
4. Modify SQLite repository query with `WHERE created_at < ?` and `LIMIT`
5. Calculate `has_more` by checking if count > limit
6. Return cursor as the last comment's ID

**Handler Update:**
```go
// File: apps/backend/internal/task/handlers/comment_handlers.go
func (h *CommentHandlers) httpListComments(c *gin.Context) {
  taskID := c.Param("id")
  limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))
  before := c.Query("before")
  sort := c.DefaultQuery("sort", "desc")

  resp, err := h.commentController.ListComments(c.Request.Context(), dto.ListCommentsRequest{
    TaskID: taskID,
    Limit:  limit,
    Before: before,
    Sort:   sort,
  })
  // ... return resp with has_more and cursor
}
```

**Index Check:**
```sql
-- Should already exist from schema
CREATE INDEX IF NOT EXISTS idx_comments_task_created
  ON task_comments(task_id, created_at DESC);
```

#### 1.2 Update SSR to Limit to 10

**Server Component:** `app/task/[id]/page.tsx`
```typescript
// Update to only fetch last 10 messages
comments = await listTaskComments(id, { limit: 10 }, { cache: 'no-store' });
```

**Frontend HTTP Client:** `lib/http/index.ts`
```typescript
export async function listTaskComments(
  taskId: string,
  params?: { limit?: number; before?: string; sort?: string },
  options?: ApiRequestOptions
) {
  const query = new URLSearchParams();
  if (params?.limit) query.set('limit', params.limit.toString());
  if (params?.before) query.set('before', params.before);
  if (params?.sort) query.set('sort', params.sort);

  const url = `/api/v1/tasks/${taskId}/comments${query.toString() ? `?${query}` : ''}`;
  return fetchJson<ListCommentsResponse>(url, options);
}
```

**Types Update:** `lib/types/http.ts`
```typescript
export interface ListCommentsResponse {
  comments: Comment[];
  total: number;
  has_more: boolean;  // NEW
  cursor: string;     // NEW
}
```

#### 1.3 Store Enhancements

**Zustand Store:**
```typescript
type CommentsState = {
  taskId: string | null;
  items: Comment[];  // Sorted by created_at ASC
  isLoading: boolean;
  hasMore: boolean;
  oldestCursor: string | null;  // For lazy loading

  // Actions
  setComments(taskId: string, comments: Comment[]);
  prependComments(comments: Comment[]); // For lazy load
  addComment(comment: Comment);          // For new messages
  setCommentsMetadata(meta: { hasMore?, isLoading? });
};
```

#### 1.4 Lazy Loading Hook

**`useLazyLoadComments.ts`:**
```typescript
export function useLazyLoadComments(taskId: string) {
  const { hasMore, oldestCursor, isLoading } = useStore(s => s.comments);
  const prependComments = useStore(s => s.prependComments);

  const loadMore = useCallback(async () => {
    if (!hasMore || isLoading) return;

    setCommentsMetadata({ isLoading: true });

    const response = await fetch(
      `/api/tasks/${taskId}/comments?limit=20&before=${oldestCursor}`
    );
    const { comments, has_more, cursor } = await response.json();

    prependComments(comments);
    setCommentsMetadata({
      hasMore: has_more,
      oldestCursor: cursor,
      isLoading: false
    });
  }, [taskId, hasMore, oldestCursor, isLoading]);

  return { loadMore, hasMore, isLoading };
}
```

---

### Phase 2: Smart Auto-Scroll + Lazy Load Detection (Week 1)

#### 2.1 Enhance Existing Virtual List

**Update:** `task-chat-panel.tsx` (already has TanStack Virtual at line 256)

**Add Lazy Load Detection:**
```typescript
// In TaskChatPanel component, after virtualizer setup (line 261)
const { loadMore, hasMore, isLoading } = useLazyLoadComments(taskId);

// Detect scroll to top for lazy loading
useEffect(() => {
  const [firstItem] = virtualizer.getVirtualItems();
  if (!firstItem) return;

  // If first visible item is index 0 and we have more
  if (firstItem.index === 0 && hasMore && !isLoading) {
    loadMore();
  }
}, [virtualizer.getVirtualItems(), hasMore, isLoading]);

// Show loading indicator at top when loading more
const showLoadingIndicator = isLoading && hasMore;
```

#### 2.2 Smart Auto-Scroll

**Replace Existing Auto-Scroll Logic** (currently at line 263-267 in `task-chat-panel.tsx`):

```typescript
// Add ref to track if user was at bottom
const wasAtBottomRef = useRef(true);

// Check if at bottom on scroll
const checkAtBottom = useCallback(() => {
  const element = messagesContainerRef.current;
  if (!element) return;

  const { scrollTop, scrollHeight, clientHeight } = element;
  wasAtBottomRef.current = scrollHeight - scrollTop - clientHeight < 50;
}, []);

// Add scroll listener
useEffect(() => {
  const element = messagesContainerRef.current;
  if (!element) return;

  element.addEventListener('scroll', checkAtBottom);
  return () => element.removeEventListener('scroll', checkAtBottom);
}, [checkAtBottom]);

// REPLACE existing auto-scroll effect (line 264-267)
// OLD: Always scrolls to bottom
// NEW: Only scroll if user was at bottom
useEffect(() => {
  if (itemCount === 0) return;

  if (wasAtBottomRef.current) {
    virtualizer.scrollToIndex(itemCount - 1, {
      align: 'end',
      behavior: 'smooth',
    });
  }
}, [itemCount, virtualizer]);
```

#### 2.3 Maintain Scroll Position on Prepend

**Strategy:**
```typescript
// Before prepending new messages
const scrollElement = parentRef.current;
const oldScrollHeight = scrollElement.scrollHeight;
const oldScrollTop = scrollElement.scrollTop;

// Prepend messages to store
prependComments(newComments);

// After render, restore scroll position
requestAnimationFrame(() => {
  const newScrollHeight = scrollElement.scrollHeight;
  scrollElement.scrollTop = oldScrollTop + (newScrollHeight - oldScrollHeight);
});
```

---

### Phase 3: Component Architecture (Week 1-2)

#### 3.1 Component Hierarchy

```
TaskChatPanel (refactored)
├── TaskChatHeader
│   ├── Agent selector
│   ├── Settings (thinking, diff view)
│   └── Loading indicator
├── TaskChatMessages (TanStack Virtual)
│   ├── Lazy load indicator (top)
│   └── MessageRenderer (type router)
│       ├── UserMessage
│       ├── AssistantMessage + ToolCallList
│       ├── ThinkingMessage
│       ├── ToolCallCard (recursive for sub-agents)
│       ├── TodoListCard
│       ├── DiffCard
│       ├── ErrorCard
│       └── SystemMessage
└── TaskChatInput (CodeMirror with commands)
```

#### 3.2 Message Components

**Key Components:**

1. **MessageRenderer** - Type router based on `comment.type`
2. **UserMessage** - Avatar, timestamp, markdown content
3. **AssistantMessage** - Avatar, duration, tokens, copy button, markdown + tool calls
4. **ToolCallCard** - Collapsible, status badge, nested sub-agents, specialized renderers
5. **TodoListCard** - Progress bar, status icons, animated updates
6. **DiffCard** - File stats, expandable, uses @git-diff-view/react
7. **ErrorCard** - Error icon, message, stack trace
8. **ThinkingMessage** - Italic, bordered, toggle visibility
9. **SystemMessage** - Session info, warnings, status updates

**Shared Components:**
- `MessageHeader` - Avatar, name, timestamp, metadata
- `CopyButton` - Copy message content
- `ExpandButton` - Expand/collapse long content
- `StatusBadge` - Status indicators with animations

#### 3.3 State Management

**Expansion State (Zustand):**
```typescript
type UIState = {
  expandedTools: Set<string>;
  expandedMessages: Set<string>;
  showThinking: boolean;
  diffViewMode: 'unified' | 'split';

  toggleToolExpanded(id: string): void;
  toggleMessageExpanded(id: string): void;
};
```

**Persistence:**
- Store expansion state in localStorage via Zustand persist
- Restore on mount
- Clear on task change

---

### Phase 4: CodeMirror Input with Commands (Week 2)

#### 4.1 CodeMirror Setup

**Chat Input Configuration:**
```typescript
import { EditorView, basicSetup } from '@codemirror/basic-setup';
import { markdown } from '@codemirror/lang-markdown';
import { autocompletion } from '@codemirror/autocomplete';
import { keymap } from '@codemirror/view';

const chatExtensions = [
  basicSetup({ lineNumbers: false }), // Minimal for chat
  markdown(),
  autocompletion({ override: [commandCompletions, mentionCompletions] }),
  keymap.of([{ key: "Mod-Enter", run: handleSubmit }]),
  EditorView.theme({
    "&": { maxHeight: "300px", minHeight: "60px" },
    ".cm-content": { padding: "12px" },
    ".cm-scroller": { overflow: "auto" }
  })
];
```

**Features:**
- Markdown mode with syntax awareness
- Slash commands (/read, /edit, /bash, etc.)
- File mentions (@filename.ts)
- Syntax highlighting for inline code
- Cmd+Enter to submit
- Auto-grow up to 300px height

#### 4.2 Command Autocomplete

**Command Completion Source:**
```typescript
import { CompletionContext } from '@codemirror/autocomplete';

const COMMANDS = [
  { label: '/read', detail: 'Read a file', info: 'Usage: /read <file_path>' },
  { label: '/edit', detail: 'Edit a file', info: 'Usage: /edit <file_path>' },
  { label: '/bash', detail: 'Run bash command', info: 'Usage: /bash <command>' },
  { label: '/grep', detail: 'Search in files', info: 'Usage: /grep <pattern>' },
  { label: '/help', detail: 'Show commands', info: 'Display all available commands' },
];

function commandCompletions(context: CompletionContext) {
  const word = context.matchBefore(/\/\w*/);
  if (!word || (word.from === word.to && !context.explicit)) return null;

  return {
    from: word.from,
    options: COMMANDS.map(cmd => ({
      label: cmd.label,
      detail: cmd.detail,
      info: cmd.info,
      apply: cmd.label + ' ', // Add space after command
    }))
  };
}
```

**File Mention Completion:**
```typescript
function mentionCompletions(context: CompletionContext) {
  const word = context.matchBefore(/@[\w\/\-\.]*/);
  if (!word) return null;

  // Get available files from git status or file tree
  const files = getChangedFiles(); // From store or context

  return {
    from: word.from,
    options: files.map(file => ({
      label: `@${file.path}`,
      detail: file.type,
      info: `Reference ${file.path}`,
      apply: `@${file.path}`,
    }))
  };
}
```

**Keyboard Navigation:**
- Built-in: Arrow up/down, Enter, Escape
- Tab to accept completion
- Fuzzy matching included

#### 4.3 Submit Handler

**Extract Content and Send:**
```typescript
function handleSubmit(view: EditorView): boolean {
  const content = view.state.doc.toString();

  if (!content.trim()) return false;

  // Parse commands (optional - backend can also parse)
  const commands = extractCommands(content);

  // Send to backend
  client.request('comment.add', {
    task_id: taskId,
    content,
    metadata: commands.length > 0 ? { commands } : undefined
  });

  // Clear editor
  view.dispatch({
    changes: { from: 0, to: view.state.doc.length, insert: '' }
  });

  return true;
}
```

#### 4.4 File Editing Integration

**Unified Editor for Multiple Contexts:**

**1. Chat Input (Minimal):**
```typescript
const chatExtensions = [
  basicSetup({ lineNumbers: false }),
  markdown(),
  autocompletion({ override: [commandCompletions, mentionCompletions] }),
];
```

**2. File Viewer (Read-only):**
```typescript
const viewerExtensions = [
  basicSetup(),
  EditorView.editable.of(false),
  EditorState.readOnly.of(true),
  languageForFile(filename),
  syntaxHighlighting(oneDarkHighlightStyle),
];
```

**3. File Editor (Full-featured):**
```typescript
const editorExtensions = [
  basicSetup(),
  languageForFile(filename),
  syntaxHighlighting(oneDarkHighlightStyle),
  lintGutter(),
  autocompletion(),
  EditorView.updateListener.of((update) => {
    if (update.docChanged) {
      debouncedSave(update.state.doc.toString());
    }
  }),
];
```

**Language Detection:**
```typescript
function languageForFile(filename: string) {
  const ext = filename.split('.').pop();
  switch (ext) {
    case 'js':
    case 'jsx':
    case 'ts':
    case 'tsx':
      return javascript({ jsx: true, typescript: ext.includes('ts') });
    case 'py':
      return python();
    case 'go':
      return go();
    case 'md':
      return markdown();
    // ... 100+ languages supported
    default:
      return [];
  }
}
```

**Shared Configuration:**
```typescript
// Reusable theme and settings
export const baseTheme = EditorView.theme({
  "&": { height: "100%" },
  ".cm-scroller": { fontFamily: "var(--font-mono)" },
  ".cm-gutters": { backgroundColor: "var(--muted)" },
});

export const baseExtensions = [
  baseTheme,
  EditorState.tabSize.of(2),
  EditorState.allowMultipleSelections.of(true),
];
```

---

### Phase 5: Rich Content Rendering (Week 2-3)

#### 5.1 Tool Call Enhancements

**Features:**
- Recursive rendering of sub-agent calls (Task tool)
- Tool-specific summaries (Read: filename, Bash: command preview)
- Status badges with animations (pending, running, success, error)
- Collapsible params and results
- Copy button for results
- Specialized renderers (Bash output with ANSI colors, JSON pretty-print)

**Metadata Structure:**
```typescript
type ToolCallMetadata = {
  tool: {
    id: string;
    name: string;
    args: Record<string, any>;
    result: string | null;
    status: 'pending' | 'running' | 'success' | 'error';
    duration_ms?: number;
    sub_agent_calls?: ToolCallMetadata[];
  };
};
```

#### 5.2 Todo List Visualization

**Features:**
- Progress bar (completed / total)
- Status icons (Circle → Loader → CheckCircle)
- Active form vs content based on status
- Animated transitions
- Group by status (optional)

**Backend Integration:**
- Agent sends `progress` type comments
- Metadata contains todos array
- Updates sent as new comments (immutable)
- Frontend shows latest state

#### 5.3 Diff Integration

**Features:**
- Extract diffs from tool call results (Edit, Write tools)
- Show file stats (additions, deletions)
- Expandable diff viewer using @git-diff-view/react
- Syntax highlighting per language
- "Open in Changes" button → navigate to Changes tab

**Diff Sources:**
1. Tool result with `diff` field
2. Tool result with `old_content` and `new_content`
3. Unified diff string in result

#### 5.4 Thinking Display

**Features:**
- Toggle via settings
- Italic text with left border
- Markdown rendering
- Collapsed by default for long thoughts
- Visual distinction from regular messages

**Backend:**
- Agent sends `thinking` type comments
- Or `metadata.thinking` in content comments

#### 5.5 Error Handling

**Features:**
- Error cards with red border
- Error icon and title
- Error message
- Collapsible stack trace
- Retry button (if applicable)
- Copy error details

---

### Phase 6: Performance & Polish (Week 3-4)

#### 6.1 Performance Optimizations

**Memoization:**
```typescript
// Memoize message components
const MessageItem = memo(MessageRenderer, (prev, next) => {
  return prev.comment.id === next.comment.id &&
         prev.comment.updated_at === next.comment.updated_at;
});

// Memoize filtered comments
const visibleComments = useMemo(() => {
  return comments.filter(c => {
    if (c.type === 'thinking' && !showThinking) return false;
    return true;
  });
}, [comments, showThinking]);
```

**Debouncing:**
- Scroll position checks: 100ms
- Lazy load triggers: 200ms
- Auto-scroll: 50ms
- Input changes: 300ms (for autocomplete)

**Code Splitting:**
```typescript
// Lazy load heavy components
const MonacoDiffEditor = lazy(() => import('./MonacoDiffEditor'));
const MermaidRenderer = lazy(() => import('./MermaidRenderer'));
```

**Bundle Size:**
- Target: ~150KB increase (CodeMirror core + essential languages)
- Use tree-shaking
- Import only needed language modes
- Lazy load additional languages on demand

#### 6.2 UX Polish

**Interactions:**
- Smooth height transitions (CSS transitions)
- Hover states on interactive elements
- Loading skeletons for lazy load
- Copy feedback (checkmark animation)
- Keyboard shortcuts (j/k navigation, / for command palette)

**Visual:**
- Consistent spacing and alignment
- Color-coded message types
- Status indicators with meaningful colors
- Avatar fallbacks with initials
- Timestamp formatting (relative times)
- Duration formatting (ms, s, m)

**Accessibility:**
- Keyboard navigation
- Focus management in autocomplete


#### 6.3 Error Boundaries

**Wrap Critical Components:**
```typescript
<ErrorBoundary
  fallback={<ErrorFallback />}
  onError={(error) => console.error('Message render error:', error)}
>
  <MessageRenderer comment={comment} />
</ErrorBoundary>
```

**Graceful Degradation:**
- If markdown fails, show plain text
- If diff parsing fails, show unified diff string
- If tool metadata is invalid, show raw JSON

---

## Backend Changes

### API Enhancements

**New Endpoints:**
```
GET  /api/tasks/:id/comments?limit=10&before=:cursor&sort=desc
POST /api/tasks/:id/comments (existing, no changes)
```

**Response Format:**
```json
{
  "comments": [
    {
      "id": "cmt_123",
      "task_id": "tsk_456",
      "author_type": "agent",
      "content": "...",
      "type": "content",
      "metadata": {...},
      "created_at": "2025-01-14T12:00:00Z"
    }
  ],
  "has_more": true,
  "cursor": "cmt_100"
}
```

**Indexes:**
```sql
CREATE INDEX IF NOT EXISTS idx_comments_task_created
  ON task_comments(task_id, created_at DESC);
```

### WebSocket (No Changes)

Existing `comment.added` notification works as-is.

### Comment Type Usage

**Agent Integration:**
- `message` - User messages
- `content` - Agent text responses
- `tool_call` - Tool executions
- `progress` - Todo list updates
- `thinking` - Agent reasoning
- `error` - Error messages
- `status` - System notifications

---

## File Structure

```
apps/web/
├── app/task/[id]/
│   ├── page.tsx                    (SSR: fetch initial comments)
│   └── page-client.tsx             (Client: hydrate + subscribe)
├── components/task/
│   ├── task-chat-panel.tsx         (Main container, refactored)
│   └── chat/
│       ├── TaskChatHeader.tsx
│       ├── TaskChatMessages.tsx    (TanStack Virtual)
│       ├── TaskChatInput.tsx       (Lexical + commands)
│       ├── messages/
│       │   ├── MessageRenderer.tsx
│       │   ├── UserMessage.tsx
│       │   ├── AssistantMessage.tsx
│       │   ├── ThinkingMessage.tsx
│       │   ├── SystemMessage.tsx
│       │   └── ErrorCard.tsx
│       ├── tools/
│       │   ├── ToolCallCard.tsx
│       │   ├── ToolStatusBadge.tsx
│       │   └── ToolIcon.tsx
│       ├── content/
│       │   ├── TodoListCard.tsx
│       │   ├── TodoItem.tsx
│       │   ├── DiffCard.tsx
│       │   └── CopyButton.tsx
│       ├── input/
│       │   ├── CodeMirrorEditor.tsx
│       │   ├── commandCompletions.ts
│       │   ├── mentionCompletions.ts
│       │   └── editorExtensions.ts
│       └── hooks/
│           ├── useLazyLoadComments.ts
│           ├── useSmartAutoScroll.ts
│           └── useChatVirtualizer.ts
└── lib/
    ├── state/store.ts              (Enhanced with pagination)
    └── api/comments.ts             (New: fetch comments API)

apps/backend/
└── internal/task/
    ├── handlers/
    │   └── comment_handlers.go     (Add pagination support)
    └── repository/
        └── sqlite.go               (Cursor-based queries)
```

**Total new files:** ~25
**Modified files:** 4 (page.tsx, page-client.tsx, task-chat-panel.tsx, store.ts, comment_handlers.go, sqlite.go)

---

## Implementation Timeline

### Week 1: Pagination & Smart Scroll
- [x] ~~SSR setup~~ (Already done)
- [x] ~~TanStack Virtual~~ (Already done v3.13.18)
- [x] ~~Basic tool calls~~ (Already done)
- [ ] Backend pagination API (limit, before, cursor)
- [ ] Update SSR to limit to 10 messages
- [ ] Lazy loading on scroll up
- [ ] Smart auto-scroll (only if at bottom)
- [ ] Scroll position preservation on prepend

### Week 2: Rich Content
- [ ] Enhanced message components (copy, timestamps)
- [ ] Nested tool call rendering (sub-agents)
- [ ] Todo list visualization with progress bars
- [ ] Inline diff viewer integration
- [ ] Thinking message display
- [ ] Error card enhancements

### Week 3: CodeMirror Input
- [ ] Replace Textarea with CodeMirror
- [ ] Markdown mode with syntax highlighting
- [ ] Slash command autocomplete (/read, /edit, /bash)
- [ ] File mention autocomplete (@file.ts)
- [ ] Keyboard navigation (arrows, enter, escape)
- [ ] Command parsing & execution
- [ ] File viewer/editor config (bonus)

### Week 4: Polish & Testing
- [ ] Performance optimization (memoization audit)
- [ ] Error boundaries for message rendering
- [ ] Accessibility (keyboard nav)
- [ ] Documentation updates

---
