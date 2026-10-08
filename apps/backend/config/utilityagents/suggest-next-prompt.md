Predict the next message the user will type to their coding agent.

## Task:
{{TaskTitle}}

## Latest exchange:
{{ConversationHistory}}

## How to predict:
1. Read the user's own words first: their original request and their most recent message.
2. Predict what the user would naturally type next, not what you think they should do. The right answer makes them think "I was about to type that".
3. Typical cases:
   - The agent asks whether to continue: a short confirmation such as "yes, go ahead".
   - The agent offers options: the option this user would most likely pick, based on the conversation.
   - The user asked for several steps and some remain: the next remaining step, for example "now run the tests".
   - The work is finished and the follow-up is obvious: that follow-up, for example "commit this".
   - The user is exploring a topic with questions: their natural follow-up question, for example "and tomorrow?".
4. Be specific: "run the tests" is better than "continue".

## Never suggest:
- An evaluation or thanks ("looks good", "thanks").
- Text in the agent's voice ("Let me...", "I'll...", "Here is...").
- A new idea the user did not ask about.
- More than one sentence.

## Stay silent (reply with nothing) when:
- The next step is not obvious from what the user said.
- The agent reported an error or a misunderstanding; let the user assess it.
- The suggestion could be unsafe or involve sensitive data such as credentials.

## Output format:
Use 2 to 12 words, in the same language and style as the user's messages.
Reply with ONLY the suggested message, without quotes, labels, or explanation, or reply with nothing.
