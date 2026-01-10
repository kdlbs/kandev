package queue

import (
	"container/heap"
	"errors"
	"sync"
	"time"

	v1 "github.com/kandev/kandev/pkg/api/v1"
)

var (
	// ErrQueueFull is returned when the queue is at max capacity
	ErrQueueFull = errors.New("queue is full")
	// ErrTaskExists is returned when a task already exists in the queue
	ErrTaskExists = errors.New("task already exists in queue")
)

// QueuedTask represents a task in the priority queue
type QueuedTask struct {
	TaskID    string
	Priority  int       // Higher priority = processed first
	AgentType string
	QueuedAt  time.Time
	Task      *v1.Task // Full task data
	index     int      // Index in the heap (used by container/heap)
}

// taskHeap implements heap.Interface for priority queue
type taskHeap []*QueuedTask

func (h taskHeap) Len() int { return len(h) }

func (h taskHeap) Less(i, j int) bool {
	// Higher priority first, then earlier queued time
	if h[i].Priority != h[j].Priority {
		return h[i].Priority > h[j].Priority
	}
	return h[i].QueuedAt.Before(h[j].QueuedAt)
}

func (h taskHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *taskHeap) Push(x interface{}) {
	n := len(*h)
	item := x.(*QueuedTask)
	item.index = n
	*h = append(*h, item)
}

func (h *taskHeap) Pop() interface{} {
	old := *h
	n := len(old)
	item := old[n-1]
	old[n-1] = nil  // avoid memory leak
	item.index = -1 // for safety
	*h = old[0 : n-1]
	return item
}

// TaskQueue manages the priority queue of tasks
type TaskQueue struct {
	mu      sync.RWMutex
	heap    taskHeap
	taskMap map[string]*QueuedTask // For quick lookup by task ID
	maxSize int
}

// NewTaskQueue creates a new task queue
func NewTaskQueue(maxSize int) *TaskQueue {
	q := &TaskQueue{
		heap:    make(taskHeap, 0),
		taskMap: make(map[string]*QueuedTask),
		maxSize: maxSize,
	}
	heap.Init(&q.heap)
	return q
}

// Enqueue adds a task to the queue
// Returns error if queue is full or task already exists
func (q *TaskQueue) Enqueue(task *v1.Task) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.taskMap[task.ID]; exists {
		return ErrTaskExists
	}

	if q.maxSize > 0 && len(q.heap) >= q.maxSize {
		return ErrQueueFull
	}

	agentType := ""
	if task.AgentType != nil {
		agentType = *task.AgentType
	}

	qt := &QueuedTask{
		TaskID:    task.ID,
		Priority:  task.Priority,
		AgentType: agentType,
		QueuedAt:  time.Now(),
		Task:      task,
	}

	heap.Push(&q.heap, qt)
	q.taskMap[task.ID] = qt
	return nil
}

// Dequeue removes and returns the highest priority task
// Returns nil if queue is empty
func (q *TaskQueue) Dequeue() *QueuedTask {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.heap) == 0 {
		return nil
	}

	qt := heap.Pop(&q.heap).(*QueuedTask)
	delete(q.taskMap, qt.TaskID)
	return qt
}

// Peek returns the highest priority task without removing it
func (q *TaskQueue) Peek() *QueuedTask {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.heap) == 0 {
		return nil
	}
	return q.heap[0]
}

// Remove removes a specific task from the queue
func (q *TaskQueue) Remove(taskID string) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	qt, exists := q.taskMap[taskID]
	if !exists {
		return false
	}

	heap.Remove(&q.heap, qt.index)
	delete(q.taskMap, taskID)
	return true
}

// UpdatePriority updates the priority of a task in the queue
func (q *TaskQueue) UpdatePriority(taskID string, newPriority int) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	qt, exists := q.taskMap[taskID]
	if !exists {
		return false
	}

	qt.Priority = newPriority
	heap.Fix(&q.heap, qt.index)
	return true
}

// Contains checks if a task is in the queue
func (q *TaskQueue) Contains(taskID string) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	_, exists := q.taskMap[taskID]
	return exists
}

// Len returns the number of tasks in the queue
func (q *TaskQueue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return len(q.heap)
}

// IsFull returns true if the queue is at max capacity
func (q *TaskQueue) IsFull() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return q.maxSize > 0 && len(q.heap) >= q.maxSize
}

// List returns all queued tasks (for status endpoint)
func (q *TaskQueue) List() []*QueuedTask {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]*QueuedTask, len(q.heap))
	copy(result, q.heap)
	return result
}

// Clear removes all tasks from the queue
func (q *TaskQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.heap = make(taskHeap, 0)
	q.taskMap = make(map[string]*QueuedTask)
	heap.Init(&q.heap)
}

