package graph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// MockTask implements the Task interface for testing
type MockTask struct {
	id           string
	hash         string
	dependencies []Task
	files        []string
	executeFunc  func(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult
}

func NewMockTask(id, hash string, deps []Task) *MockTask {
	return &MockTask{
		id:           id,
		hash:         hash,
		dependencies: deps,
		files:        []string{fmt.Sprintf("%s.txt", id)},
	}
}

func (m *MockTask) ID() string {
	return m.id
}

func (m *MockTask) Hash() string {
	return m.hash
}

func (m *MockTask) Dependencies() []Task {
	return m.dependencies
}

func (m *MockTask) Execute(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult {
	if m.executeFunc != nil {
		return m.executeFunc(ctx, workDir, dependencyInputs)
	}
	
	// Default implementation: create a file
	filename := filepath.Join(workDir, fmt.Sprintf("%s.txt", m.id))
	content := fmt.Sprintf("Output from task %s", m.id)
	
	// Include dependency information in the output
	for _, dep := range dependencyInputs {
		content += fmt.Sprintf("\nUsing dependency %s from %s with files: %v", dep.TaskID, dep.OutputDir, dep.Files)
	}
	
	if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
		return TaskResult{Error: err}
	}
	
	return TaskResult{Files: []string{fmt.Sprintf("%s.txt", m.id)}}
}

func TestGraph_AddTask(t *testing.T) {
	graph := NewGraph()
	task := NewMockTask("task1", "hash1", nil)
	
	err := graph.AddTask(task)
	if err != nil {
		t.Fatalf("Expected no error adding task, got: %v", err)
	}
	
	// Try adding the same task again
	err = graph.AddTask(task)
	if err == nil {
		t.Fatal("Expected error when adding duplicate task")
	}
}

func TestGraph_TopologicalSort(t *testing.T) {
	graph := NewGraph()
	
	// Create tasks: A -> B -> C (A depends on B, B depends on C)
	taskC := NewMockTask("C", "hashC", nil)
	taskB := NewMockTask("B", "hashB", []Task{taskC})
	taskA := NewMockTask("A", "hashA", []Task{taskB})
	
	graph.AddTask(taskA)
	graph.AddTask(taskB)
	graph.AddTask(taskC)
	
	sorted, err := graph.TopologicalSort()
	if err != nil {
		t.Fatalf("Expected no error in topological sort, got: %v", err)
	}
	
	if len(sorted) != 3 {
		t.Fatalf("Expected 3 tasks, got %d", len(sorted))
	}
	
	// C should come first (no dependencies), then B, then A
	if sorted[0].ID() != "C" {
		t.Errorf("Expected first task to be C, got %s", sorted[0].ID())
	}
	if sorted[1].ID() != "B" {
		t.Errorf("Expected second task to be B, got %s", sorted[1].ID())
	}
	if sorted[2].ID() != "A" {
		t.Errorf("Expected third task to be A, got %s", sorted[2].ID())
	}
}

func TestGraph_CycleDetection(t *testing.T) {
	graph := NewGraph()
	
	// Create a cycle: A -> B -> A
	taskA := NewMockTask("A", "hashA", nil)
	taskB := NewMockTask("B", "hashB", []Task{taskA})
	taskA.dependencies = []Task{taskB} // Create the cycle
	
	graph.AddTask(taskA)
	graph.AddTask(taskB)
	
	_, err := graph.TopologicalSort()
	if err == nil {
		t.Fatal("Expected error for cyclic graph")
	}
}

func TestComputeTaskHash(t *testing.T) {
	taskC := NewMockTask("C", "hashC", nil)
	taskB := NewMockTask("B", "hashB", []Task{taskC})
	taskA := NewMockTask("A", "hashA", []Task{taskB})
	
	hashA := ComputeTaskHash(taskA)
	hashB := ComputeTaskHash(taskB)
	hashC := ComputeTaskHash(taskC)
	
	// Hash should include dependencies, so they should all be different
	if hashA == hashB || hashB == hashC || hashA == hashC {
		t.Error("Expected different hashes for tasks with different dependencies")
	}
	
	// Hash should be consistent
	hashA2 := ComputeTaskHash(taskA)
	if hashA != hashA2 {
		t.Error("Expected consistent hash for same task")
	}
}

func TestRunner_Execute(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	graph := NewGraph()
	runner := NewRunner(tempDir)
	
	// Create tasks: A depends on B, B depends on C
	taskC := NewMockTask("C", "hashC", nil)
	taskB := NewMockTask("B", "hashB", []Task{taskC})
	taskA := NewMockTask("A", "hashA", []Task{taskB})
	
	graph.AddTask(taskA)
	graph.AddTask(taskB)
	graph.AddTask(taskC)
	
	ctx := context.Background()
	results, err := runner.Execute(ctx, graph)
	if err != nil {
		t.Fatalf("Expected no error in execution, got: %v", err)
	}
	
	if len(results) != 3 {
		t.Fatalf("Expected 3 results, got %d", len(results))
	}
	
	// Verify execution order (C, B, A)
	if results[0].Task.ID() != "C" {
		t.Errorf("Expected first result to be C, got %s", results[0].Task.ID())
	}
	if results[1].Task.ID() != "B" {
		t.Errorf("Expected second result to be B, got %s", results[1].Task.ID())
	}
	if results[2].Task.ID() != "A" {
		t.Errorf("Expected third result to be A, got %s", results[2].Task.ID())
	}
	
	// Verify files were created
	for _, result := range results {
		if result.Result.Error != nil {
			t.Errorf("Task %s failed: %v", result.Task.ID(), result.Result.Error)
		}
		
		// Check that output directory exists
		if _, err := os.Stat(result.OutputDir); os.IsNotExist(err) {
			t.Errorf("Output directory %s does not exist", result.OutputDir)
		}
		
		// Check that task file was created
		taskFile := filepath.Join(result.OutputDir, fmt.Sprintf("%s.txt", result.Task.ID()))
		if _, err := os.Stat(taskFile); os.IsNotExist(err) {
			t.Errorf("Task file %s does not exist", taskFile)
		}
	}
}

func TestRunner_ExecuteWithFailure(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	graph := NewGraph()
	runner := NewRunner(tempDir)
	
	// Create a task that will fail
	failingTask := NewMockTask("failing", "hashFail", nil)
	failingTask.executeFunc = func(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult {
		return TaskResult{Error: fmt.Errorf("task failed")}
	}
	
	graph.AddTask(failingTask)
	
	ctx := context.Background()
	_, err = runner.Execute(ctx, graph)
	if err == nil {
		t.Fatal("Expected error when task fails")
	}
}

func TestRunner_ExecuteWithCancellation(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	graph := NewGraph()
	runner := NewRunner(tempDir)
	
	// Create a task that takes time
	slowTask := NewMockTask("slow", "hashSlow", nil)
	slowTask.executeFunc = func(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult {
		select {
		case <-time.After(100 * time.Millisecond):
			return TaskResult{Files: []string{"slow.txt"}}
		case <-ctx.Done():
			return TaskResult{Error: ctx.Err()}
		}
	}
	
	graph.AddTask(slowTask)
	
	ctx, cancel := context.WithCancel(context.Background())
	
	// Cancel immediately
	cancel()
	
	_, err = runner.Execute(ctx, graph)
	if err == nil {
		t.Fatal("Expected error when context is cancelled")
	}
}

func TestRunner_Caching(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_cache_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	graph := NewGraph()
	runner := NewRunner(tempDir)
	
	// Create a simple task
	executionCount := 0
	cachedTask := NewMockTask("cached", "hashCached", nil)
	cachedTask.executeFunc = func(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult {
		executionCount++
		// Create a file
		filename := filepath.Join(workDir, "cached.txt")
		content := fmt.Sprintf("Execution #%d", executionCount)
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return TaskResult{Error: err}
		}
		return TaskResult{Files: []string{"cached.txt"}}
	}
	
	graph.AddTask(cachedTask)
	
	ctx := context.Background()
	
	// First execution should run the task
	results1, err := runner.Execute(ctx, graph)
	if err != nil {
		t.Fatalf("First execution failed: %v", err)
	}
	
	if len(results1) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results1))
	}
	
	if results1[0].CacheHit {
		t.Error("First execution should not be a cache hit")
	}
	
	if executionCount != 1 {
		t.Errorf("Expected 1 execution, got %d", executionCount)
	}
	
	// Second execution should use cache
	results2, err := runner.Execute(ctx, graph)
	if err != nil {
		t.Fatalf("Second execution failed: %v", err)
	}
	
	if len(results2) != 1 {
		t.Fatalf("Expected 1 result, got %d", len(results2))
	}
	
	if !results2[0].CacheHit {
		t.Error("Second execution should be a cache hit")
	}
	
	if executionCount != 1 {
		t.Errorf("Expected still 1 execution after cache hit, got %d", executionCount)
	}
	
	// Verify the cached files are the same
	if results1[0].TaskHash != results2[0].TaskHash {
		t.Error("Task hash should be consistent between executions")
	}
	
	if results1[0].OutputDir != results2[0].OutputDir {
		t.Error("Output directory should be the same for cached results")
	}
}

func TestRunner_DependencyInputs(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "graph_deps_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	graph := NewGraph()
	runner := NewRunner(tempDir)
	
	// Create dependency task
	depTask := NewMockTask("dependency", "hashDep", nil)
	
	// Create main task that depends on depTask
	mainTask := NewMockTask("main", "hashMain", []Task{depTask})
	var receivedInputs []DependencyInput
	mainTask.executeFunc = func(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult {
		receivedInputs = dependencyInputs
		
		// Create output file
		filename := filepath.Join(workDir, "main.txt")
		content := "Main task output"
		if err := os.WriteFile(filename, []byte(content), 0644); err != nil {
			return TaskResult{Error: err}
		}
		return TaskResult{Files: []string{"main.txt"}}
	}
	
	graph.AddTask(depTask)
	graph.AddTask(mainTask)
	
	ctx := context.Background()
	results, err := runner.Execute(ctx, graph)
	if err != nil {
		t.Fatalf("Execution failed: %v", err)
	}
	
	if len(results) != 2 {
		t.Fatalf("Expected 2 results, got %d", len(results))
	}
	
	// Verify dependency inputs were passed correctly
	if len(receivedInputs) != 1 {
		t.Fatalf("Expected 1 dependency input, got %d", len(receivedInputs))
	}
	
	depInput := receivedInputs[0]
	if depInput.TaskID != "dependency" {
		t.Errorf("Expected dependency task ID 'dependency', got '%s'", depInput.TaskID)
	}
	
	if len(depInput.Files) != 1 || depInput.Files[0] != "dependency.txt" {
		t.Errorf("Expected dependency files ['dependency.txt'], got %v", depInput.Files)
	}
	
	// Verify output directory is set correctly
	if depInput.OutputDir == "" {
		t.Error("Dependency output directory should not be empty")
	}
}