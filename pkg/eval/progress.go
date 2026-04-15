package eval

// ProgressCallback is called during eval execution to report progress.
// Implementations must be safe for concurrent use when running with --parallel > 1,
// as callbacks may be invoked from multiple goroutines simultaneously.
type ProgressCallback func(event ProgressEvent)

// ProgressEvent represents a progress update during eval execution
type ProgressEvent struct {
	Type    ProgressEventType
	Message string
	Task    *EvalResult  // Populated for task-related events
	Summary *EvalSummary // Populated for EventEvalStart
}

// ProgressEventType represents the type of progress event
type ProgressEventType string

const (
	EventEvalStart      ProgressEventType = "eval_start"
	EventTaskStart      ProgressEventType = "task_start"
	EventTaskSetup      ProgressEventType = "task_setup"
	EventTaskRunning    ProgressEventType = "task_running"
	EventTaskVerifying  ProgressEventType = "task_verifying"
	EventTaskAssertions ProgressEventType = "task_assertions"
	EventTaskComplete   ProgressEventType = "task_complete"
	EventTaskTimeout    ProgressEventType = "task_timeout"
	EventTaskError      ProgressEventType = "task_error"
	EventEvalComplete   ProgressEventType = "eval_complete"
)

// NoopProgressCallback is a progress callback that does nothing
func NoopProgressCallback(event ProgressEvent) {
	// No-op
}
