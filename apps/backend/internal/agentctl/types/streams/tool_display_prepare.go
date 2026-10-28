package streams

import (
	"encoding/json"
	"strings"
	"unicode/utf8"

	"github.com/kandev/kandev/internal/agentctl/types/deliverylimits"
)

// toolTruncationSuffix marks a bounded text carrier. It is ordinary text in an
// existing content block, not a new metadata namespace or wire field.
const toolTruncationSuffix = " … [truncated by Kandev]"

const toolOmissionNotice = "[Kandev omitted oversized tool display data]"

const (
	toolDisplayItemNodes = 256
	toolDisplayMutations = 64
	toolDisplayFiles     = 512
	toolDisplaySteps     = 256
	toolDisplayTodos     = 512
	toolIdentityBytes    = 4 << 10
	// displayStructureReserve keeps JSON envelope overhead outside the
	// aggregate display accounting so a fully-used budget still fits.
	displayStructureReserve = 4 << 10
)

// displayBudget tracks the aggregate encoded display data of one tool event.
// It bounds allocation and traversal; the journal independently enforces the
// complete event size before sequence allocation.
type displayBudget struct {
	remaining int
}

func newDisplayBudget(limit int64) displayBudget {
	return displayBudget{remaining: int(normalizeDisplayLimit(limit) - displayStructureReserve)}
}

func normalizeDisplayLimit(limit int64) int64 {
	if limit <= 0 || limit > deliverylimits.DisplayBytes {
		return deliverylimits.DisplayBytes
	}
	return limit
}

func (b *displayBudget) charge(bytes int) bool {
	if bytes <= 0 {
		return true
	}
	if bytes > b.remaining {
		b.remaining = 0
		return false
	}
	b.remaining -= bytes
	return true
}

// text copies a string within the budget and appends the explicit marker when
// the value was reduced. The marker cost is reserved inside the same budget so
// repeat preparation accepts the prepared bytes unchanged.
func (b *displayBudget) text(value string) (string, bool) {
	return b.boundedString(value, b.remaining)
}

// identity copies a small identity field under a fixed per-field cap so a
// pathological control string cannot consume the display budget.
func (b *displayBudget) identity(value string) (string, bool) {
	return b.boundedString(value, min(b.remaining, toolIdentityBytes))
}

func (b *displayBudget) boundedString(value string, limit int) (string, bool) {
	if limit <= 0 {
		return "", value != ""
	}
	trial := limit
	if bounded, cut := copyToolString(value, &trial); !cut {
		b.remaining -= limit - trial
		return bounded, false
	}
	markerCost := toolEscapedStringCost(toolTruncationSuffix)
	body := limit - markerCost
	if body < 2 {
		b.remaining -= min(limit, markerCost)
		return toolTruncationSuffix, true
	}
	copyBudget := body
	bounded, _ := copyToolString(value, &copyBudget)
	b.remaining -= limit - copyBudget
	return bounded + toolTruncationSuffix, true
}

// toolEscapedStringCost returns the escaped JSON content cost of a string
// without its surrounding quotes; copyToolString charges those separately.
func toolEscapedStringCost(value string) int {
	cost := 0
	for end := 0; end < len(value); {
		r, size := utf8.DecodeRuneInString(value[end:])
		cost += toolJSONRuneBytes(r, size)
		end += size
	}
	return cost
}

// displayCost accumulates escaped JSON cost without serializing the source
// value. It stops at the remaining budget so an oversized provider body costs
// at most O(budget) work and no full escaped/base64 allocation.
type displayCost struct {
	total    int
	limit    int
	exceeded bool
}

func newDisplayCost(limit int) displayCost {
	return displayCost{limit: limit}
}

func (c *displayCost) add(value int) {
	if c.exceeded {
		return
	}
	c.total += value
	if c.total > c.limit {
		c.exceeded = true
	}
}

func (c *displayCost) addString(value string) {
	if c.exceeded {
		return
	}
	c.add(2)
	for end := 0; end < len(value); {
		r, size := utf8.DecodeRuneInString(value[end:])
		c.add(toolJSONRuneBytes(r, size))
		end += size
		if c.exceeded {
			return
		}
	}
}

func mutationDisplayCost(mutation FileMutation, limit int) displayCost {
	cost := newDisplayCost(limit)
	cost.add(64)
	cost.addString(string(mutation.Type))
	cost.addString(mutation.Content)
	cost.addString(mutation.OldContent)
	cost.addString(mutation.NewContent)
	cost.addString(mutation.Diff)
	cost.addString(mutation.NewPath)
	return cost
}

func todoDisplayCost(item TodoItem, limit int) displayCost {
	cost := newDisplayCost(limit)
	cost.add(32)
	cost.addString(item.ID)
	cost.addString(item.Description)
	cost.addString(item.Status)
	cost.addString(item.ActiveForm)
	return cost
}

func toolContentDisplayCost(item ToolCallContentItem, limit int) displayCost {
	cost := newDisplayCost(limit)
	cost.add(32)
	cost.addString(item.Type)
	cost.addString(item.Path)
	if item.OldText != nil {
		cost.addString(*item.OldText)
	}
	cost.addString(item.NewText)
	cost.addString(item.TerminalID)
	if item.Content != nil {
		cost.addContentBlock(*item.Content)
	}
	return cost
}

func (c *displayCost) addContentBlock(block ContentBlock) {
	c.add(8)
	c.addString(block.Type)
	c.addString(block.Text)
	c.addString(block.Data)
	c.addString(block.MimeType)
	c.addString(block.URI)
	c.addString(block.Name)
	c.addString(block.Title)
	c.addString(block.Description)
}

func (b *displayBudget) encoded(value any) int {
	encoded, err := json.Marshal(value)
	if err != nil {
		return b.remaining
	}
	return len(encoded)
}

// PrepareToolEvent applies the shared all-category display policy with the
// default aggregate budget. Accepted events are immutable: the source payload
// and content carriers are never mutated.
func PrepareToolEvent(event AgentEvent) AgentEvent {
	return PrepareToolEventWithLimit(event, deliverylimits.DisplayBytes)
}

// PrepareToolEventWithLimit prepares one tool event for a configured event
// limit. The limit reserves delivery envelope space before display data is
// accepted.
func PrepareToolEventWithLimit(event AgentEvent, limit int64) AgentEvent {
	if event.Type != EventTypeToolCall && event.Type != EventTypeToolUpdate {
		return event
	}
	if event.NormalizedPayload == nil && len(event.ToolCallContents) == 0 {
		return event
	}
	budget := newDisplayBudget(limit)
	// Reserve the omission notice and generic preview costs before the richer
	// carriers consume the aggregate budget. A reserved item is not charged
	// again when iteration encounters it, so repeat preparation is identical.
	noticeCost := toolNoticeCost()
	budget.charge(noticeCost)
	originalContents := event.ToolCallContents
	if genericContentOnly(event) && len(originalContents) > 0 {
		budget.charge(toolPreviewBytes*6 + 128)
	}
	event, omitted := prepareToolDisplay(event, &budget)
	contents, reduced := prepareToolContents(originalContents, &budget, omitted)
	event.ToolCallContents = contents
	if reduced && genericContentOnly(event) {
		event.NormalizedPayload = withGenericContentPreview(event.NormalizedPayload, originalContents)
	}
	if encoded, err := json.Marshal(event); err == nil && int64(len(encoded)) > normalizeDisplayLimit(limit) {
		return tightenToolEvent(event)
	}
	return event
}

// genericContentOnly preserves the existing generic `{truncated, preview}`
// representation when a content-only result is reduced and the typed output is
// empty. Existing generic renderers keep working without new metadata.
func genericContentOnly(event AgentEvent) bool {
	return event.NormalizedPayload != nil && event.NormalizedPayload.kind == ToolKindGeneric &&
		event.NormalizedPayload.generic != nil && event.NormalizedPayload.generic.Output == nil
}

func withGenericContentPreview(payload *NormalizedPayload, contents []ToolCallContentItem) *NormalizedPayload {
	clone := *payload
	generic := *payload.generic
	generic.Output = truncatedToolValue(toolContentsPreview(contents))
	clone.generic = &generic
	return &clone
}

func prepareToolDisplay(event AgentEvent, budget *displayBudget) (AgentEvent, bool) {
	omitted := false
	if event.NormalizedPayload != nil {
		payload, dropped := prepareNormalizedPayload(event.NormalizedPayload, budget)
		event.NormalizedPayload = payload
		omitted = dropped
	}
	return event, omitted
}

func prepareNormalizedPayload(payload *NormalizedPayload, budget *displayBudget) (*NormalizedPayload, bool) {
	clone := *payload
	omitted := false
	switch payload.kind {
	case ToolKindReadFile:
		clone.readFile = prepareReadFile(payload.readFile, budget)
	case ToolKindModifyFile:
		clone.modifyFile, omitted = prepareModifyFile(payload.modifyFile, budget)
	case ToolKindShellExec:
		clone.shellExec = prepareShellExec(payload.shellExec, budget)
	case ToolKindCodeSearch:
		clone.codeSearch = prepareCodeSearch(payload.codeSearch, budget)
	case ToolKindHttpRequest:
		clone.httpRequest = prepareHttpRequest(payload.httpRequest, budget)
	case ToolKindGeneric:
		clone.generic, omitted = prepareGeneric(payload.generic, budget)
	case ToolKindCreateTask:
		clone.createTask = prepareCreateTask(payload.createTask, budget)
	case ToolKindSubagentTask:
		clone.subagentTask = prepareSubagentTask(payload.subagentTask, budget)
	case ToolKindShowPlan:
		clone.showPlan, omitted = prepareShowPlan(payload.showPlan, budget)
	case ToolKindManageTodos:
		clone.manageTodos, omitted = prepareManageTodos(payload.manageTodos, budget)
	case ToolKindMisc:
		clone.misc, omitted = prepareMisc(payload.misc, budget)
	}
	return &clone, omitted
}

func prepareReadFile(source *ReadFilePayload, budget *displayBudget) *ReadFilePayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.FilePath)
	budget.charge(32)
	if source.Output != nil {
		output := *source.Output
		budget.identity(output.Language)
		budget.charge(32)
		text, cut := budget.text(output.Content)
		output.Content = text
		output.Truncated = output.Truncated || cut
		clone.Output = &output
	}
	return &clone
}

func prepareModifyFile(source *ModifyFilePayload, budget *displayBudget) (*ModifyFilePayload, bool) {
	if source == nil {
		return nil, false
	}
	clone := *source
	budget.identity(source.FilePath)
	budget.charge(32)
	clone.Mutations = nil
	for index, mutation := range source.Mutations {
		if index >= toolDisplayMutations {
			return &clone, true
		}
		cost := mutationDisplayCost(mutation, budget.remaining)
		// A partial file body or diff would misrepresent executable content, so
		// an oversized body-bearing entry is omitted as a whole.
		if cost.exceeded {
			return &clone, true
		}
		budget.charge(cost.total)
		clone.Mutations = append(clone.Mutations, mutation)
	}
	return &clone, len(clone.Mutations) != len(source.Mutations)
}

func prepareShellExec(source *ShellExecPayload, budget *displayBudget) *ShellExecPayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.Command)
	budget.identity(source.WorkDir)
	budget.identity(source.Description)
	budget.charge(64)
	if source.Output != nil {
		// The authoritative stdout/stderr tails and their normalization remain
		// unchanged; only their small bookkeeping fields are carried here.
		output := *source.Output
		clone.Output = &output
		budget.charge(64)
	}
	return &clone
}

func prepareCodeSearch(source *CodeSearchPayload, budget *displayBudget) *CodeSearchPayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.Query)
	budget.identity(source.Pattern)
	budget.identity(source.Path)
	budget.identity(source.Glob)
	budget.charge(32)
	if source.Output == nil {
		return &clone
	}
	output := *source.Output
	output.Files = nil
	for index, file := range source.Output.Files {
		if index >= toolDisplayFiles {
			output.Truncated = true
			break
		}
		text, cut := budget.identity(file)
		if cut || text == "" {
			output.Truncated = true
			break
		}
		output.Files = append(output.Files, text)
	}
	if len(output.Files) != len(source.Output.Files) {
		output.Truncated = true
	}
	// Keep the supplied total. Never invent a count the provider did not send.
	clone.Output = &output
	return &clone
}

func prepareHttpRequest(source *HttpRequestPayload, budget *displayBudget) *HttpRequestPayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.URL)
	budget.identity(source.Method)
	budget.charge(32)
	response, _ := budget.text(source.Response)
	clone.Response = response
	return &clone
}

func prepareGeneric(source *GenericPayload, budget *displayBudget) (*GenericPayload, bool) {
	if source == nil {
		return nil, false
	}
	clone := *source
	budget.identity(source.Name)
	clone.Input = budget.genericValue(source.Input)
	clone.Output = budget.genericValue(source.Output)
	omitted := (source.Input != nil && clone.Input == nil) || (source.Output != nil && clone.Output == nil)
	return &clone, omitted
}

// genericValue retains the existing 64 KiB generic JSON policy and then
// charges the bounded value against the aggregate display budget. A value that
// still cannot fit is reduced to the existing truncation marker.
func (b *displayBudget) genericValue(value any) any {
	if value == nil {
		return nil
	}
	bounded := boundToolValue(value)
	if cost := b.encoded(bounded); cost <= b.remaining {
		b.charge(cost)
		return bounded
	}
	bounded = truncatedToolValue(value)
	if cost := b.encoded(bounded); cost <= b.remaining {
		b.charge(cost)
		return bounded
	}
	return nil
}

func prepareCreateTask(source *CreateTaskPayload, budget *displayBudget) *CreateTaskPayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.Title)
	description, _ := budget.text(source.Description)
	clone.Description = description
	return &clone
}

func prepareSubagentTask(source *SubagentTaskPayload, budget *displayBudget) *SubagentTaskPayload {
	if source == nil {
		return nil
	}
	clone := *source
	budget.identity(source.Description)
	budget.identity(source.SubagentType)
	budget.identity(source.AgentID)
	budget.identity(source.Model)
	budget.identity(source.ChildSessionID)
	budget.identity(source.ProviderThreadID)
	budget.identity(source.OutputFile)
	budget.charge(128)
	prompt, _ := budget.text(source.Prompt)
	result, _ := budget.text(source.ResultText)
	clone.Prompt = prompt
	clone.ResultText = result
	return &clone
}

func prepareShowPlan(source *ShowPlanPayload, budget *displayBudget) (*ShowPlanPayload, bool) {
	if source == nil {
		return nil, false
	}
	clone := *source
	summary, summaryCut := budget.text(source.Summary)
	clone.Summary = summary
	clone.Steps = nil
	for index, step := range source.Steps {
		if index >= toolDisplaySteps {
			break
		}
		text, cut := budget.text(step)
		if cut && text == "" {
			break
		}
		clone.Steps = append(clone.Steps, text)
	}
	omitted := summaryCut || len(clone.Steps) != len(source.Steps)
	return &clone, omitted
}

func prepareManageTodos(source *ManageTodosPayload, budget *displayBudget) (*ManageTodosPayload, bool) {
	if source == nil {
		return nil, false
	}
	clone := *source
	clone.Items = nil
	for index, item := range source.Items {
		if index >= toolDisplayTodos {
			break
		}
		cost := todoDisplayCost(item, budget.remaining)
		if cost.exceeded {
			break
		}
		budget.charge(cost.total)
		clone.Items = append(clone.Items, item)
	}
	return &clone, len(clone.Items) != len(source.Items)
}

func prepareMisc(source *MiscPayload, budget *displayBudget) (*MiscPayload, bool) {
	if source == nil {
		return nil, false
	}
	clone := *source
	budget.identity(source.Label)
	details := boundToolValue(source.Details)
	budget.charge(budget.encoded(details))
	clone.Details = details
	return &clone, false
}

func prepareToolContents(contents []ToolCallContentItem, budget *displayBudget, omitted bool) ([]ToolCallContentItem, bool) {
	if len(contents) == 0 {
		if omitted {
			return []ToolCallContentItem{toolNoticeContent(toolOmissionNotice)}, true
		}
		return contents, false
	}
	capacity := len(contents)
	if capacity > toolDisplayItemNodes+1 {
		capacity = toolDisplayItemNodes + 1
	}
	result := make([]ToolCallContentItem, 0, capacity)
	dropped := false
	for index, item := range contents {
		if index >= toolDisplayItemNodes {
			dropped = true
			break
		}
		bounded, keep, cut := prepareToolContent(item, budget)
		dropped = dropped || cut
		if !keep {
			dropped = true
			continue
		}
		result = append(result, bounded)
	}
	if dropped || omitted {
		result = append(result, toolNoticeContent(toolOmissionNotice))
	}
	if len(result) == 0 {
		return nil, true
	}
	return result, dropped || omitted
}

func prepareToolContent(item ToolCallContentItem, budget *displayBudget) (ToolCallContentItem, bool, bool) {
	if item.Type == "content" && item.Content != nil && item.Content.Text == toolOmissionNotice {
		// The omission notice is reserved once at admission; replaying it must
		// not consume additional budget or append a duplicate.
		return cloneToolContentItem(item), true, false
	}
	budget.charge(32)
	switch item.Type {
	case "content":
		return prepareContentBlock(item, budget)
	case "diff":
		cost := toolContentDisplayCost(item, budget.remaining)
		if cost.exceeded {
			return ToolCallContentItem{}, false, true
		}
		budget.charge(cost.total)
		return cloneToolContentItem(item), true, false
	case "terminal":
		budget.identity(item.TerminalID)
		return cloneToolContentItem(item), true, false
	default:
		cost := toolContentDisplayCost(item, budget.remaining)
		if cost.exceeded {
			return ToolCallContentItem{}, false, true
		}
		budget.charge(cost.total)
		return cloneToolContentItem(item), true, false
	}
}

func prepareContentBlock(item ToolCallContentItem, budget *displayBudget) (ToolCallContentItem, bool, bool) {
	if item.Content == nil {
		return cloneToolContentItem(item), true, false
	}
	block := *item.Content
	if block.Size != nil {
		size := *block.Size
		block.Size = &size
	}
	switch block.Type {
	case "image", "audio", "resource":
		cost := newDisplayCost(budget.remaining)
		cost.addContentBlock(block)
		if cost.exceeded {
			// Never present a cut binary or resource block as valid content.
			return ToolCallContentItem{}, false, true
		}
		budget.charge(cost.total)
		item.Content = &block
		return cloneToolContentItem(item), true, false
	case "text":
		text, cut := budget.text(block.Text)
		block.Text = text
		item.Content = &block
		return cloneToolContentItem(item), true, cut
	default:
		block.Text, _ = budget.text(block.Text)
		block.URI, _ = budget.identity(block.URI)
		block.Name, _ = budget.identity(block.Name)
		block.Title, _ = budget.identity(block.Title)
		item.Content = &block
		return cloneToolContentItem(item), true, false
	}
}

func toolNoticeContent(text string) ToolCallContentItem {
	return ToolCallContentItem{Type: "content", Content: &ContentBlock{Type: "text", Text: text}}
}

// toolNoticeCost is the conservative encoded cost of the omission item.
func toolNoticeCost() int {
	return len(toolOmissionNotice) + 128
}

func cloneToolContentItem(item ToolCallContentItem) ToolCallContentItem {
	clone := item
	if item.Content != nil {
		block := *item.Content
		if block.Size != nil {
			size := *block.Size
			block.Size = &size
		}
		clone.Content = &block
	}
	if item.OldText != nil {
		text := *item.OldText
		clone.OldText = &text
	}
	return clone
}

// tightenToolEvent is the final bounded fallback. It drops display bodies once
// and leaves an explicit marker in the existing content carrier. Control and
// identity fields are preserved so the journal can return a typed size error
// if the envelope is still too large.
func tightenToolEvent(event AgentEvent) AgentEvent {
	if event.NormalizedPayload != nil {
		event.NormalizedPayload = tightenNormalizedPayload(event.NormalizedPayload)
	}
	event.ToolCallContents = []ToolCallContentItem{toolNoticeContent(toolOmissionNotice)}
	return event
}

func tightenNormalizedPayload(payload *NormalizedPayload) *NormalizedPayload {
	clone := *payload
	switch clone.kind {
	case ToolKindReadFile:
		clone.readFile = nil
	case ToolKindModifyFile:
		clone.modifyFile = tightenModifyFilePayload(clone.modifyFile)
	case ToolKindCodeSearch:
		clone.codeSearch = tightenCodeSearchPayload(clone.codeSearch)
	case ToolKindHttpRequest:
		clone.httpRequest = tightenHttpRequestPayload(clone.httpRequest)
	case ToolKindGeneric:
		clone.generic = tightenGenericPayload(clone.generic)
	case ToolKindSubagentTask:
		clone.subagentTask = tightenSubagentPayload(clone.subagentTask)
	case ToolKindShowPlan:
		clone.showPlan = tightenShowPlanPayload(clone.showPlan)
	case ToolKindManageTodos:
		clone.manageTodos = tightenManageTodosPayload(clone.manageTodos)
	case ToolKindMisc:
		clone.misc = tightenMiscPayload(clone.misc)
	}
	return &clone
}

func tightenModifyFilePayload(payload *ModifyFilePayload) *ModifyFilePayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Mutations = nil
	return &clone
}

func tightenCodeSearchPayload(payload *CodeSearchPayload) *CodeSearchPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	if clone.Output != nil {
		output := *clone.Output
		output.Files = nil
		output.Truncated = true
		clone.Output = &output
	}
	return &clone
}

func tightenHttpRequestPayload(payload *HttpRequestPayload) *HttpRequestPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Response = toolOmissionNotice
	return &clone
}

func tightenGenericPayload(payload *GenericPayload) *GenericPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Input = nil
	clone.Output = map[string]any{"truncated": true, "preview": ""}
	return &clone
}

func tightenSubagentPayload(payload *SubagentTaskPayload) *SubagentTaskPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Prompt = toolOmissionNotice
	clone.ResultText = toolOmissionNotice
	return &clone
}

func tightenShowPlanPayload(payload *ShowPlanPayload) *ShowPlanPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Steps = nil
	return &clone
}

func tightenManageTodosPayload(payload *ManageTodosPayload) *ManageTodosPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Items = nil
	return &clone
}

func tightenMiscPayload(payload *MiscPayload) *MiscPayload {
	if payload == nil {
		return nil
	}
	clone := *payload
	clone.Details = nil
	return &clone
}

// IsToolDisplayPrepared reports whether a text carrier already holds the
// library-owned truncation marker. Tests use it to prove repeat preparation.
func IsToolDisplayPrepared(value string) bool {
	return strings.HasSuffix(value, toolTruncationSuffix)
}
