package graph

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
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
type ProgressCallback func(task Task, status string, finished bool, cached bool)

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
	return r.ExecuteWithProgressParallel(ctx, graph, progressCallback, 1)
}

// ExecuteWithProgressParallel runs all tasks in the graph with progress callbacks using parallel workers
func (r *Runner) ExecuteWithProgressParallel(ctx context.Context, graph *Graph, progressCallback ProgressCallback, parallelWorkers int) ([]ExecutionResult, error) {
	if parallelWorkers <= 1 {
		// Fall back to sequential execution
		return r.executeSequential(ctx, graph, progressCallback)
	}
	
	return r.executeParallel(ctx, graph, progressCallback, parallelWorkers)
}

// executeSequential runs tasks sequentially (original implementation)
func (r *Runner) executeSequential(ctx context.Context, graph *Graph, progressCallback ProgressCallback) ([]ExecutionResult, error) {
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
			progressCallback(task, "running", false, false)
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
			progressCallback(task, status, true, result.CacheHit)
		}
		
		// Stop execution if task failed
		if result.Result.Error != nil {
			return results, fmt.Errorf("task %s failed: %w", task.ID(), result.Result.Error)
		}
	}
	
	return results, nil
}

// executeParallel runs tasks in parallel using worker goroutines
func (r *Runner) executeParallel(ctx context.Context, graph *Graph, progressCallback ProgressCallback, parallelWorkers int) ([]ExecutionResult, error) {
	allTasks := graph.GetTasks()
	
	// Track task dependencies and completion status
	taskDeps := make(map[string]map[string]bool) // taskID -> set of dependency task IDs
	taskInDegree := make(map[string]int)         // taskID -> number of uncompleted dependencies
	
	// Initialize dependency tracking
	for _, task := range allTasks {
		taskID := task.ID()
		deps := make(map[string]bool)
		for _, dep := range task.Dependencies() {
			deps[dep.ID()] = true
		}
		taskDeps[taskID] = deps
		taskInDegree[taskID] = len(deps)
	}
	
	// Channels for communication
	taskQueue := make(chan Task, len(allTasks))
	resultChan := make(chan ExecutionResult, len(allTasks))
	errorChan := make(chan error, parallelWorkers)
	
	// Shared executed tasks map with mutex for thread safety
	executedTasks := &SafeExecutedTasks{
		tasks: make(map[string]ExecutionResult),
	}
	
	// Add tasks with no dependencies to the initial queue
	for _, task := range allTasks {
		if taskInDegree[task.ID()] == 0 {
			select {
			case taskQueue <- task:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}
	
	// Start worker goroutines
	for i := 0; i < parallelWorkers; i++ {
		go r.workerParallel(ctx, taskQueue, resultChan, errorChan, progressCallback, executedTasks)
	}
	
	// Collect results and manage task queue
	var results []ExecutionResult
	completedCount := 0
	
	for completedCount < len(allTasks) {
		select {
		case <-ctx.Done():
			return results, ctx.Err()
		case err := <-errorChan:
			return results, err
		case result := <-resultChan:
			// Handle task completion
			results = append(results, result)
			executedTasks.Set(result.Task.ID(), result)
			completedCount++
			
			// Stop execution if task failed
			if result.Result.Error != nil {
				return results, fmt.Errorf("task %s failed: %w", result.Task.ID(), result.Result.Error)
			}
			
			// Update dependency counts and queue newly available tasks
			completedTaskID := result.Task.ID()
			for _, task := range allTasks {
				taskID := task.ID()
				if deps, exists := taskDeps[taskID]; exists {
					if deps[completedTaskID] {
						// This task was waiting for the completed task
						taskInDegree[taskID]--
						if taskInDegree[taskID] == 0 {
							// All dependencies are now complete, queue this task
							select {
							case taskQueue <- task:
							case <-ctx.Done():
								return results, ctx.Err()
							}
						}
					}
				}
			}
		}
	}
	
	// Close the task queue to signal workers to stop
	close(taskQueue)
	
	return results, nil
}

// SafeExecutedTasks provides thread-safe access to executed tasks
type SafeExecutedTasks struct {
	tasks map[string]ExecutionResult
	mu    sync.RWMutex
}

func (s *SafeExecutedTasks) Set(taskID string, result ExecutionResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tasks[taskID] = result
}

func (s *SafeExecutedTasks) ToMap() map[string]ExecutionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	
	// Create a copy of the map
	result := make(map[string]ExecutionResult)
	for k, v := range s.tasks {
		result[k] = v
	}
	return result
}

// workerParallel executes tasks from the queue with access to shared executed tasks
func (r *Runner) workerParallel(ctx context.Context, taskQueue <-chan Task, resultChan chan<- ExecutionResult, errorChan chan<- error, progressCallback ProgressCallback, executedTasks *SafeExecutedTasks) {
	for {
		select {
		case <-ctx.Done():
			return
		case task, ok := <-taskQueue:
			if !ok {
				return // Channel closed, worker should exit
			}
			
			// Process the task
			if progressCallback != nil {
				progressCallback(task, "running", false, false)
			}
			
			// Get current executed tasks for dependency resolution
			currentExecutedTasks := executedTasks.ToMap()
			
			result, err := r.executeTask(ctx, task, currentExecutedTasks)
			if err != nil {
				select {
				case errorChan <- fmt.Errorf("failed to execute task %s: %w", task.ID(), err):
				case <-ctx.Done():
				}
				return
			}
			
			// Notify progress callback
			if progressCallback != nil {
				status := "completed"
				if result.Result.Error != nil {
					status = "failed"
				}
				progressCallback(task, status, true, result.CacheHit)
			}
			
			// Send result
			select {
			case resultChan <- result:
			case <-ctx.Done():
				return
			}
		}
	}
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
	
	// Create temporary directory for task execution
	tempDir, err := os.MkdirTemp("", "fbs-task-")
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir) // Always clean up temp directory
	
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
	
	// Execute the task in the temporary directory
	taskResult := task.Execute(ctx, tempDir, dependencyInputs)
	
	// Only move to cache if the task succeeded
	if taskResult.Error == nil {
		// Create the final cache directory
		err := os.MkdirAll(outputDir, 0755)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("failed to create cache directory %s: %w", outputDir, err)
		}
		
		// Move contents from temp directory to cache directory
		err = r.moveTempToCache(tempDir, outputDir)
		if err != nil {
			return ExecutionResult{}, fmt.Errorf("failed to move temp results to cache: %w", err)
		}
	}
	// If task failed, temp directory will be cleaned up by defer
	
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
	// Walk the output directory to find all files (including subdirectories)
	var files []string
	err := filepath.Walk(outputDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			// Get relative path from the output directory
			relPath, err := filepath.Rel(outputDir, path)
			if err != nil {
				return err
			}
			files = append(files, relPath)
		}
		return nil
	})
	
	if err != nil {
		return ExecutionResult{}, fmt.Errorf("failed to walk cached output directory: %w", err)
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

// moveTempToCache moves all contents from temp directory to cache directory
func (r *Runner) moveTempToCache(tempDir, cacheDir string) error {
	// Walk through all files in temp directory
	return filepath.Walk(tempDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Get relative path from temp directory
		relPath, err := filepath.Rel(tempDir, path)
		if err != nil {
			return err
		}
		
		// Skip the root directory itself
		if relPath == "." {
			return nil
		}
		
		// Destination path in cache directory
		destPath := filepath.Join(cacheDir, relPath)
		
		if info.IsDir() {
			// Create directory in cache
			return os.MkdirAll(destPath, info.Mode())
		} else {
			// Create parent directory if needed
			if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
				return err
			}
			
			// Move file from temp to cache
			return os.Rename(path, destPath)
		}
	})
}

// ExecuteTask executes a single task (useful for testing or selective execution)
func (r *Runner) ExecuteTask(ctx context.Context, task Task) (ExecutionResult, error) {
	return r.executeTask(ctx, task, make(map[string]ExecutionResult))
}