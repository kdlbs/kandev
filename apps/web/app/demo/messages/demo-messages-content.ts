export const MARKDOWN_DEMO_MESSAGE_CONTENT = `**[MARKDOWN DEMO]** Here's a message showing all markdown elements:

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
\`\`\``;
