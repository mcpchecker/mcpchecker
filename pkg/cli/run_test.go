package cli

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mcpchecker/mcpchecker/pkg/eval"
)

func TestProgressDisplay_EventHandling(t *testing.T) {
	tests := []struct {
		name          string
		events        []eval.ProgressEvent
		wantRunning   int
		wantPassed    int
		wantFailed    int
		wantTotal     int
		wantStarted   bool
		wantFinished  bool
	}{
		{
			name: "eval start sets total tasks",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 12}, // 3 tasks * 4 phases
			},
			wantTotal: 3,
		},
		{
			name: "task start increments running",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 4},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task1"}},
			},
			wantRunning: 1,
			wantTotal:   1,
		},
		{
			name: "task complete decrements running and increments passed",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 4},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task1"}},
				{Type: eval.EventTaskComplete, Task: &eval.EvalResult{TaskName: "task1", TaskPassed: true}},
			},
			wantRunning: 0,
			wantPassed:  1,
			wantFailed:  0,
			wantTotal:   1,
		},
		{
			name: "task error decrements running and increments failed",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 4},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task1"}},
				{Type: eval.EventTaskError, Task: &eval.EvalResult{TaskName: "task1", TaskPassed: false}},
			},
			wantRunning: 0,
			wantPassed:  0,
			wantFailed:  1,
			wantTotal:   1,
		},
		{
			name: "multiple tasks in parallel",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 12},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task1"}},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task2"}},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task3"}},
				{Type: eval.EventTaskComplete, Task: &eval.EvalResult{TaskName: "task1", TaskPassed: true}},
				{Type: eval.EventTaskComplete, Task: &eval.EvalResult{TaskName: "task2", TaskPassed: true}},
				{Type: eval.EventTaskError, Task: &eval.EvalResult{TaskName: "task3", TaskPassed: false}},
			},
			wantRunning: 0,
			wantPassed:  2,
			wantFailed:  1,
			wantTotal:   3,
		},
		{
			name: "eval complete sets finished flag",
			events: []eval.ProgressEvent{
				{Type: eval.EventEvalStart, TotalTasks: 4},
				{Type: eval.EventTaskStart, Task: &eval.EvalResult{TaskName: "task1"}},
				{Type: eval.EventTaskComplete, Task: &eval.EvalResult{TaskName: "task1", TaskPassed: true}},
				{Type: eval.EventEvalComplete},
			},
			wantRunning:  0,
			wantPassed:   1,
			wantFailed:   0,
			wantTotal:    1,
			wantFinished: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newProgressDisplay(false)

			for _, event := range tt.events {
				d.handleProgress(event)
			}

			if d.running != tt.wantRunning {
				t.Errorf("running = %d, want %d", d.running, tt.wantRunning)
			}
			if d.passed != tt.wantPassed {
				t.Errorf("passed = %d, want %d", d.passed, tt.wantPassed)
			}
			if d.failed != tt.wantFailed {
				t.Errorf("failed = %d, want %d", d.failed, tt.wantFailed)
			}
			if d.total != tt.wantTotal {
				t.Errorf("total = %d, want %d", d.total, tt.wantTotal)
			}
			if d.finished != tt.wantFinished {
				t.Errorf("finished = %v, want %v", d.finished, tt.wantFinished)
			}

			if d.ticker != nil && !d.finished {
				d.ticker.Stop()
				close(d.stopTicker)
			}
		})
	}
}

func TestProgressDisplay_ThreadSafety(t *testing.T) {
	d := newProgressDisplay(false)

	// Simulate concurrent progress events like in parallel task execution
	var wg sync.WaitGroup
	events := []eval.ProgressEvent{
		{Type: eval.EventEvalStart, TotalTasks: 40},
	}

	d.handleProgress(events[0])

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(taskNum int) {
			defer wg.Done()

			taskName := string(rune('A' + taskNum))

			d.handleProgress(eval.ProgressEvent{
				Type: eval.EventTaskStart,
				Task: &eval.EvalResult{TaskName: taskName},
			})

			time.Sleep(10 * time.Millisecond)

			d.handleProgress(eval.ProgressEvent{
				Type: eval.EventTaskComplete,
				Task: &eval.EvalResult{TaskName: taskName, TaskPassed: true},
			})
		}(i)
	}

	wg.Wait()

	if d.running != 0 {
		t.Errorf("running = %d, want 0", d.running)
	}
	if d.passed != 10 {
		t.Errorf("passed = %d, want 10", d.passed)
	}
	if d.failed != 0 {
		t.Errorf("failed = %d, want 0", d.failed)
	}

	d.handleProgress(eval.ProgressEvent{Type: eval.EventEvalComplete})
}

func TestProgressDisplay_RenderProgress(t *testing.T) {
	// Redirect stderr to capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	d := newProgressDisplay(false)
	d.startTime = time.Now()
	d.running = 3
	d.passed = 5
	d.failed = 1
	d.total = 10

	d.renderProgress()

	// Restore stderr
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check that output contains expected values
	expectedStrings := []string{
		"Running: 3",
		"Passed: 5",
		"Failed: 1",
		"Completed: 6/10",
		"Elapsed:",
	}

	for _, expected := range expectedStrings {
		if !strings.Contains(output, expected) {
			t.Errorf("output missing %q, got: %q", expected, output)
		}
	}
}

func TestProgressDisplay_RenderProgress_WhenFinished(t *testing.T) {
	// Redirect stderr to capture output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	d := newProgressDisplay(false)
	d.finished = true
	d.startTime = time.Now()

	d.renderProgress()

	// Restore stderr
	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Should produce no output when finished
	if output != "" {
		t.Errorf("expected no output when finished, got: %q", output)
	}
}

func TestProgressDisplay_SpinnerAnimation(t *testing.T) {
	d := newProgressDisplay(false)
	d.startTime = time.Now()

	// Wait a bit to change the spinner frame
	time.Sleep(150 * time.Millisecond)

	// Redirect stderr
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w

	d.renderProgress()

	w.Close()
	os.Stderr = oldStderr

	var buf bytes.Buffer
	buf.ReadFrom(r)
	output := buf.String()

	// Check that output starts with a spinner character (Braille pattern)
	if len(output) == 0 || output[0] != '\r' {
		t.Errorf("expected output to start with carriage return, got: %q", output)
	}

	// Should contain a Braille pattern character (they're multibyte UTF-8)
	if !strings.Contains(output, "⠋") &&
	   !strings.Contains(output, "⠙") &&
	   !strings.Contains(output, "⠹") &&
	   !strings.Contains(output, "⠸") &&
	   !strings.Contains(output, "⠼") &&
	   !strings.Contains(output, "⠴") &&
	   !strings.Contains(output, "⠦") &&
	   !strings.Contains(output, "⠧") &&
	   !strings.Contains(output, "⠇") &&
	   !strings.Contains(output, "⠏") {
		t.Errorf("expected spinner character in output, got: %q", output)
	}
}

func TestProgressDisplay_ResultsBuffering(t *testing.T) {
	d := newProgressDisplay(false)

	// Send events for multiple tasks
	d.handleProgress(eval.ProgressEvent{Type: eval.EventEvalStart, TotalTasks: 8})
	d.handleProgress(eval.ProgressEvent{
		Type: eval.EventTaskStart,
		Task: &eval.EvalResult{TaskName: "task1"},
	})
	d.handleProgress(eval.ProgressEvent{
		Type: eval.EventTaskStart,
		Task: &eval.EvalResult{TaskName: "task2"},
	})

	// Results should be buffered
	if len(d.results) != 2 {
		t.Errorf("results length = %d, want 2", len(d.results))
	}

	if d.results["task1"] == nil {
		t.Error("task1 not found in results")
	}
	if d.results["task2"] == nil {
		t.Error("task2 not found in results")
	}

	// Cleanup
	if d.ticker != nil {
		d.ticker.Stop()
		close(d.stopTicker)
	}
}
