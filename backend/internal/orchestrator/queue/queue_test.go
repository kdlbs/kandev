package queue

import (
	"testing"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// createTestTask creates a task for testing with the given parameters
func createTestTask(id string, priority int, agentType string) *v1.Task {
	return &v1.Task{
		ID:        id,
		BoardID:   "test-board",
		Title:     "Test Task " + id,
		Priority:  priority,
		AgentType: &agentType,
		State:     v1.TaskStateTODO,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

func TestNewTaskQueue(t *testing.T) {
	q := NewTaskQueue(100)
	if q == nil {
		t.Fatal("NewTaskQueue returned nil")
	}
	if q.Len() != 0 {
		t.Errorf("expected empty queue, got Len() = %d", q.Len())
	}
	if q.maxSize != 100 {
		t.Errorf("expected maxSize = 100, got %d", q.maxSize)
	}
}

func TestEnqueue(t *testing.T) {
	q := NewTaskQueue(10)
	task := createTestTask("task-1", 5, "test-agent")

	err := q.Enqueue(task)
	if err != nil {
		t.Fatalf("Enqueue failed: %v", err)
	}

	if q.Len() != 1 {
		t.Errorf("expected Len() = 1, got %d", q.Len())
	}
}

func TestEnqueueDuplicate(t *testing.T) {
	q := NewTaskQueue(10)
	task := createTestTask("task-1", 5, "test-agent")

	_ = q.Enqueue(task)
	err := q.Enqueue(task)
	if err != ErrTaskExists {
		t.Errorf("expected ErrTaskExists, got %v", err)
	}
}

func TestEnqueueQueueFull(t *testing.T) {
	q := NewTaskQueue(2)

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	_ = q.Enqueue(createTestTask("task-2", 5, "test-agent"))
	err := q.Enqueue(createTestTask("task-3", 5, "test-agent"))

	if err != ErrQueueFull {
		t.Errorf("expected ErrQueueFull, got %v", err)
	}
}

func TestDequeue(t *testing.T) {
	q := NewTaskQueue(10)
	task := createTestTask("task-1", 5, "test-agent")

	_ = q.Enqueue(task)
	dequeued := q.Dequeue()

	if dequeued == nil {
		t.Fatal("Dequeue returned nil")
	}
	if dequeued.TaskID != task.ID {
		t.Errorf("expected TaskID = %s, got %s", task.ID, dequeued.TaskID)
	}
	if q.Len() != 0 {
		t.Errorf("expected Len() = 0 after dequeue, got %d", q.Len())
	}
}

func TestDequeueEmptyQueue(t *testing.T) {
	q := NewTaskQueue(10)
	dequeued := q.Dequeue()
	if dequeued != nil {
		t.Errorf("expected nil from empty queue, got %v", dequeued)
	}
}

func TestPriorityOrdering(t *testing.T) {
	q := NewTaskQueue(10)

	// Enqueue tasks with different priorities
	_ = q.Enqueue(createTestTask("low", 1, "test-agent"))
	_ = q.Enqueue(createTestTask("high", 10, "test-agent"))
	_ = q.Enqueue(createTestTask("medium", 5, "test-agent"))

	// Dequeue should return highest priority first
	first := q.Dequeue()
	if first.TaskID != "high" {
		t.Errorf("expected first dequeue = 'high', got %s", first.TaskID)
	}

	second := q.Dequeue()
	if second.TaskID != "medium" {
		t.Errorf("expected second dequeue = 'medium', got %s", second.TaskID)
	}

	third := q.Dequeue()
	if third.TaskID != "low" {
		t.Errorf("expected third dequeue = 'low', got %s", third.TaskID)
	}
}

func TestPeek(t *testing.T) {
	q := NewTaskQueue(10)

	// Peek on empty queue
	peeked := q.Peek()
	if peeked != nil {
		t.Errorf("expected nil from Peek on empty queue")
	}

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	peeked = q.Peek()

	if peeked == nil {
		t.Fatal("Peek returned nil on non-empty queue")
	}
	if peeked.TaskID != "task-1" {
		t.Errorf("expected TaskID = task-1, got %s", peeked.TaskID)
	}
	// Verify it didn't remove the task
	if q.Len() != 1 {
		t.Errorf("Peek should not remove task from queue")
	}
}

func TestRemove(t *testing.T) {
	q := NewTaskQueue(10)

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	_ = q.Enqueue(createTestTask("task-2", 3, "test-agent"))

	removed := q.Remove("task-1")
	if !removed {
		t.Error("Remove should return true for existing task")
	}
	if q.Len() != 1 {
		t.Errorf("expected Len() = 1 after remove, got %d", q.Len())
	}
	if q.Contains("task-1") {
		t.Error("queue should not contain removed task")
	}
}

func TestRemoveNonExistent(t *testing.T) {
	q := NewTaskQueue(10)
	removed := q.Remove("non-existent")
	if removed {
		t.Error("Remove should return false for non-existent task")
	}
}

func TestUpdatePriority(t *testing.T) {
	q := NewTaskQueue(10)

	_ = q.Enqueue(createTestTask("task-1", 1, "test-agent"))
	_ = q.Enqueue(createTestTask("task-2", 10, "test-agent"))

	// task-2 should be first initially
	peeked := q.Peek()
	if peeked.TaskID != "task-2" {
		t.Errorf("expected task-2 first initially")
	}

	// Update task-1 to have higher priority
	updated := q.UpdatePriority("task-1", 20)
	if !updated {
		t.Error("UpdatePriority should return true for existing task")
	}

	// Now task-1 should be first
	peeked = q.Peek()
	if peeked.TaskID != "task-1" {
		t.Errorf("expected task-1 first after priority update")
	}
}

func TestUpdatePriorityNonExistent(t *testing.T) {
	q := NewTaskQueue(10)
	updated := q.UpdatePriority("non-existent", 10)
	if updated {
		t.Error("UpdatePriority should return false for non-existent task")
	}
}

func TestContains(t *testing.T) {
	q := NewTaskQueue(10)

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))

	if !q.Contains("task-1") {
		t.Error("Contains should return true for existing task")
	}
	if q.Contains("task-2") {
		t.Error("Contains should return false for non-existent task")
	}
}

func TestIsFull(t *testing.T) {
	q := NewTaskQueue(2)

	if q.IsFull() {
		t.Error("empty queue should not be full")
	}

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	if q.IsFull() {
		t.Error("queue with 1 item (capacity 2) should not be full")
	}

	_ = q.Enqueue(createTestTask("task-2", 5, "test-agent"))
	if !q.IsFull() {
		t.Error("queue at capacity should be full")
	}
}

func TestList(t *testing.T) {
	q := NewTaskQueue(10)

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	_ = q.Enqueue(createTestTask("task-2", 3, "test-agent"))
	_ = q.Enqueue(createTestTask("task-3", 7, "test-agent"))

	list := q.List()
	if len(list) != 3 {
		t.Errorf("expected List() to return 3 items, got %d", len(list))
	}
}

func TestClear(t *testing.T) {
	q := NewTaskQueue(10)

	_ = q.Enqueue(createTestTask("task-1", 5, "test-agent"))
	_ = q.Enqueue(createTestTask("task-2", 3, "test-agent"))

	q.Clear()
	if q.Len() != 0 {
		t.Errorf("expected Len() = 0 after Clear, got %d", q.Len())
	}
	if q.Contains("task-1") || q.Contains("task-2") {
		t.Error("Clear should remove all tasks")
	}
}

func TestUnlimitedQueue(t *testing.T) {
	// maxSize of 0 means unlimited
	q := NewTaskQueue(0)

	for i := 0; i < 100; i++ {
		err := q.Enqueue(createTestTask(string(rune('a'+i)), 5, "test-agent"))
		if err != nil {
			t.Fatalf("Enqueue failed on unlimited queue: %v", err)
		}
	}

	if q.IsFull() {
		t.Error("unlimited queue should never be full")
	}
}

func TestFIFOWithSamePriority(t *testing.T) {
	q := NewTaskQueue(10)

	// All tasks have same priority - should be FIFO
	_ = q.Enqueue(createTestTask("first", 5, "test-agent"))
	time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	_ = q.Enqueue(createTestTask("second", 5, "test-agent"))
	time.Sleep(1 * time.Millisecond)
	_ = q.Enqueue(createTestTask("third", 5, "test-agent"))

	first := q.Dequeue()
	if first.TaskID != "first" {
		t.Errorf("expected 'first' with FIFO ordering, got %s", first.TaskID)
	}

	second := q.Dequeue()
	if second.TaskID != "second" {
		t.Errorf("expected 'second' with FIFO ordering, got %s", second.TaskID)
	}
}

