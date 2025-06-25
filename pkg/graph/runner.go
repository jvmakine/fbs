package graph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
)

// ExecutionResult represents the result of executing a task with its output location
type ExecutionResult struct {
	Task       Task
	TaskHash   string
	OutputDir  string
	Result     TaskResult
	CacheHit   bool // Whether this result came from cache
}

// ProgressCallback is called when task execution status changes
type ProgressCallback func(task Task, status string, finished bool)

// Runner executes tasks in a graph
type Runner struct {
	resultDir string
}

// NewRunner creates a new runner that stores results in the specified directory
func NewRunner(resultDir string) *Runner {
	return &Runner{
		resultDir: resultDir,
	}
}

// Execute runs all tasks in the graph in topological order
func (r *Runner) Execute(ctx context.Context, graph *Graph) ([]ExecutionResult, error) {
	return r.ExecuteWithProgress(ctx, graph, nil)
}

// ExecuteWithProgress runs all tasks in the graph with progress callbacks
func (r *Runner) ExecuteWithProgress(ctx context.Context, graph *Graph, progressCallback ProgressCallback) ([]ExecutionResult, error) {
	// Get tasks in topological order
	orderedTasks, err := graph.TopologicalSort()
	if err != nil {
		return nil, fmt.Errorf("failed to sort tasks: %w", err)
	}
	
	var results []ExecutionResult
	executedTasks := make(map[string]ExecutionResult)
	
	for _, task := range orderedTasks {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		default:
		}
		
		// Notify progress callback that task is starting
		if progressCallback != nil {
			progressCallback(task, "running", false)
		}
		
		// Execute task
		result, err := r.executeTask(ctx, task, executedTasks)
		if err != nil {
			return results, fmt.Errorf("failed to execute task %s: %w", task.ID(), err)
		}
		
		results = append(results, result)
		executedTasks[task.ID()] = result
		
		// Notify progress callback that task is finished
		if progressCallback != nil {
			status := "completed"
			if result.Result.Error != nil {
				status = "failed"
			}
			progressCallback(task, status, true)
		}
		
		// Stop execution if task failed
		if result.Result.Error != nil {
			return results, fmt.Errorf("task %s failed: %w", task.ID(), result.Result.Error)
		}
	}
	
	return results, nil
}

// executeTask executes a single task and stores its results
func (r *Runner) executeTask(ctx context.Context, task Task, executedTasks map[string]ExecutionResult) (ExecutionResult, error) {
	// Compute task hash including dependencies
	taskHash := ComputeTaskHash(task)
	
	// Create output directory for this task
	outputDir := filepath.Join(r.resultDir, taskHash)
	
	// Check if cached result exists
	if r.isCached(outputDir) {
		// Load cached result
		cachedResult, err := r.loadCachedResult(task, taskHash, outputDir)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("failed to load cached result for task %s: %w", task.ID(), err)
		}
		return cachedResult, nil
	}
	
	// Create output directory
	err := os.MkdirAll(outputDir, 0755)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to create output directory %s: %w", outputDir, err)
	}
	
	// Gather dependency inputs
	var dependencyInputs []DependencyInput
	for _, dep := range task.Dependencies() {
		depResult, exists := executedTasks[dep.ID()]
		if !exists {
			return ExecutionResult{}, fmt.Errorf("dependency %s not found in executed tasks", dep.ID())
		}
		
		dependencyInputs = append(dependencyInputs, DependencyInput{
			TaskID:    dep.ID(),
			OutputDir: depResult.OutputDir,
			Files:     depResult.Result.Files,
		})
	}
	
	// Execute the task
	taskResult := task.Execute(ctx, outputDir, dependencyInputs)
	
	return ExecutionResult{
		Task:      task,
		TaskHash:  taskHash,
		OutputDir: outputDir,
		Result:    taskResult,
		CacheHit:  false,
	}, nil
}

// isCached checks if a cached result exists for the given output directory
func (r *Runner) isCached(outputDir string) bool {
	// Check if the output directory exists and is not empty
	if info, err := os.Stat(outputDir); err != nil || !info.IsDir() {
		return false
	}
	
	// Check if directory has any files
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return false
	}
	
	return len(entries) > 0
}

// loadCachedResult loads a cached result from the output directory
func (r *Runner) loadCachedResult(task Task, taskHash, outputDir string) (ExecutionResult, error) {
	// List files in the output directory
	entries, err := os.ReadDir(outputDir)
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to read cached output directory: %w", err)
	}
	
	var files []string
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	
	return ExecutionResult{
		Task:      task,
		TaskHash:  taskHash,
		OutputDir: outputDir,
		Result: TaskResult{
			Files: files,
			Error: nil,
		},
		CacheHit: true,
	}, nil
}

// ExecuteTask executes a single task (useful for testing or selective execution)
func (r *Runner) ExecuteTask(ctx context.Context, task Task) (ExecutionResult, error) {
	return r.executeTask(ctx, task, make(map[string]ExecutionResult))
}