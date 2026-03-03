package service

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// mockMessageRepo implements the subset of repository.MessageRepository
// needed by StreamingBuffer.
type mockMessageRepo struct {
	mu       sync.Mutex
	messages map[string]*models.Message
	appends  map[string]string // messageID → appended content
	updates  []*models.Message
}

func newMockRepo() *mockMessageRepo {
	return &mockMessageRepo{
		messages: make(map[string]*models.Message),
		appends:  make(map[string]string),
	}
}

func (m *mockMessageRepo) GetMessage(_ context.Context, id string) (*models.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[id]
	if !ok {
		return nil, fmt.Errorf("message not found: %s", id)
	}
	// Return a copy
	cp := *msg
	if msg.Metadata != nil {
		cp.Metadata = make(map[string]interface{}, len(msg.Metadata))
		for k, v := range msg.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp, nil
}

func (m *mockMessageRepo) AppendContent(_ context.Context, id, content string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	msg, ok := m.messages[id]
	if !ok {
		return fmt.Errorf("message not found: %s", id)
	}
	msg.Content += content
	m.appends[id] += content
	return nil
}

func (m *mockMessageRepo) UpdateMessage(_ context.Context, msg *models.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.messages[msg.ID]
	if !ok {
		return fmt.Errorf("message not found: %s", msg.ID)
	}
	existing.Content = msg.Content
	existing.Metadata = msg.Metadata
	m.updates = append(m.updates, msg)
	return nil
}

func (m *mockMessageRepo) CreateMessage(_ context.Context, _ *models.Message) error { return nil }
func (m *mockMessageRepo) CreateMessages(_ context.Context, _ []*models.Message) error {
	return nil
}
func (m *mockMessageRepo) GetMessageByToolCallID(_ context.Context, _, _ string) (*models.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) GetMessageByPendingID(_ context.Context, _, _ string) (*models.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) FindMessageByPendingID(_ context.Context, _ string) (*models.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) ListMessages(_ context.Context, _ string) ([]*models.Message, error) {
	return nil, nil
}
func (m *mockMessageRepo) ListMessagesPaginated(_ context.Context, _ string, _ models.ListMessagesOptions) ([]*models.Message, bool, error) {
	return nil, false, nil
}
func (m *mockMessageRepo) DeleteMessage(_ context.Context, _ string) error { return nil }

func (m *mockMessageRepo) addMessage(msg *models.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages[msg.ID] = msg
}

func (m *mockMessageRepo) getContent(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.messages[id].Content
}

func (m *mockMessageRepo) getAppendedContent(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appends[id]
}

func (m *mockMessageRepo) getUpdateCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.updates)
}

func testLogger() *logger.Logger {
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "debug", Format: "console", OutputPath: "stdout"})
	return log
}

func TestStreamingBuffer_Append(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		Content:       "Hello",
	})

	buf := NewStreamingBuffer(repo, testLogger())

	// First append: should load from DB and cache
	msg, err := buf.Append(context.Background(), "msg-1", " world")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "Hello world" {
		t.Errorf("expected 'Hello world', got %q", msg.Content)
	}

	// Second append: should use cached version (no DB read)
	msg, err = buf.Append(context.Background(), "msg-1", "!")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if msg.Content != "Hello world!" {
		t.Errorf("expected 'Hello world!', got %q", msg.Content)
	}

	// DB should still have original content (not flushed yet)
	if got := repo.getContent("msg-1"); got != "Hello" {
		t.Errorf("expected DB content 'Hello' (not flushed), got %q", got)
	}
}

func TestStreamingBuffer_Finalize(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		Content:       "base",
	})

	buf := NewStreamingBuffer(repo, testLogger())

	// Accumulate some content
	_, _ = buf.Append(context.Background(), "msg-1", " extra")
	_, _ = buf.Append(context.Background(), "msg-1", " more")

	// Finalize should flush to DB
	buf.Finalize(context.Background(), "msg-1")

	// DB should have the appended delta
	if got := repo.getAppendedContent("msg-1"); got != " extra more" {
		t.Errorf("expected appended content ' extra more', got %q", got)
	}

	// Entry should be removed from buffer
	buf.mu.Lock()
	_, exists := buf.entries["msg-1"]
	buf.mu.Unlock()
	if exists {
		t.Error("expected entry to be removed after Finalize")
	}
}

func TestStreamingBuffer_PeriodicFlush(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		Content:       "",
	})

	buf := NewStreamingBuffer(repo, testLogger())
	buf.interval = 50 * time.Millisecond
	buf.Start()
	defer buf.Stop()

	// Append content
	_, _ = buf.Append(context.Background(), "msg-1", "chunk1")
	_, _ = buf.Append(context.Background(), "msg-1", "chunk2")

	// Wait for flush
	time.Sleep(100 * time.Millisecond)

	// DB should have received the appended delta
	if got := repo.getAppendedContent("msg-1"); got != "chunk1chunk2" {
		t.Errorf("expected appended 'chunk1chunk2', got %q", got)
	}
}

func TestStreamingBuffer_AppendThinking(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-t1",
		TaskSessionID: "session-1",
		Content:       "",
		Metadata:      map[string]interface{}{"thinking": "I need to"},
	})

	buf := NewStreamingBuffer(repo, testLogger())

	msg, err := buf.AppendThinking(context.Background(), "msg-t1", " think more")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	thinking, _ := msg.Metadata["thinking"].(string)
	if thinking != "I need to think more" {
		t.Errorf("expected thinking 'I need to think more', got %q", thinking)
	}

	// Finalize should flush via UpdateMessage
	buf.Finalize(context.Background(), "msg-t1")

	if repo.getUpdateCount() != 1 {
		t.Errorf("expected 1 UpdateMessage call for thinking flush, got %d", repo.getUpdateCount())
	}
}

func TestStreamingBuffer_FinalizeAll(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-a",
		TaskSessionID: "session-1",
		Content:       "",
	})
	repo.addMessage(&models.Message{
		ID:            "msg-b",
		TaskSessionID: "session-1",
		Content:       "",
	})
	repo.addMessage(&models.Message{
		ID:            "msg-c",
		TaskSessionID: "session-2",
		Content:       "",
	})

	buf := NewStreamingBuffer(repo, testLogger())

	_, _ = buf.Append(context.Background(), "msg-a", "a-content")
	_, _ = buf.Append(context.Background(), "msg-b", "b-content")
	_, _ = buf.Append(context.Background(), "msg-c", "c-content")

	// FinalizeAll for session-1 should flush msg-a and msg-b but not msg-c
	buf.FinalizeAll(context.Background(), "session-1")

	if got := repo.getAppendedContent("msg-a"); got != "a-content" {
		t.Errorf("msg-a: expected 'a-content', got %q", got)
	}
	if got := repo.getAppendedContent("msg-b"); got != "b-content" {
		t.Errorf("msg-b: expected 'b-content', got %q", got)
	}
	if got := repo.getAppendedContent("msg-c"); got != "" {
		t.Errorf("msg-c should not be flushed yet, got %q", got)
	}

	// msg-c should still be in buffer
	buf.mu.Lock()
	_, cExists := buf.entries["msg-c"]
	buf.mu.Unlock()
	if !cExists {
		t.Error("msg-c should still be in buffer")
	}
}

func TestStreamingBuffer_ConcurrentAppends(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		Content:       "",
	})

	buf := NewStreamingBuffer(repo, testLogger())
	buf.interval = 50 * time.Millisecond
	buf.Start()
	defer buf.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = buf.Append(context.Background(), "msg-1", "x")
		}()
	}
	wg.Wait()

	// Verify in-memory content length
	buf.mu.Lock()
	entry := buf.entries["msg-1"]
	contentLen := len(entry.message.Content)
	buf.mu.Unlock()

	if contentLen != 100 {
		t.Errorf("expected 100 chars in buffer, got %d", contentLen)
	}

	// Finalize and verify DB
	buf.Finalize(context.Background(), "msg-1")
	if got := repo.getContent("msg-1"); len(got) != 100 {
		t.Errorf("expected 100 chars in DB after finalize, got %d", len(got))
	}
}

func TestStreamingBuffer_Stop(t *testing.T) {
	repo := newMockRepo()
	repo.addMessage(&models.Message{
		ID:            "msg-1",
		TaskSessionID: "session-1",
		Content:       "start",
	})

	buf := NewStreamingBuffer(repo, testLogger())
	buf.interval = 1 * time.Hour // Long interval to ensure flush happens on Stop, not timer
	buf.Start()

	_, _ = buf.Append(context.Background(), "msg-1", " end")

	// Stop should flush everything
	buf.Stop()

	if got := repo.getAppendedContent("msg-1"); got != " end" {
		t.Errorf("expected appended ' end' after Stop, got %q", got)
	}
}
