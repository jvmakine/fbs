package graph

import (
	"context"
)

// TaskType represents the type of task for filtering and execution
type TaskType string

const (
	// TaskTypeBuild represents tasks that build/compile code
	TaskTypeBuild TaskType = "build"
	// TaskTypeTest represents tasks that run tests
	TaskTypeTest TaskType = "test"
	// TaskTypeDeps represents tasks that download/manage dependencies
	TaskTypeDeps TaskType = "deps"
)

// TaskResult represents the result of executing a task
type TaskResult struct {
	// Files contains the relative paths to files produced by the task
	Files []string
	// Error contains any error that occurred during task execution
	Error error
}

// DependencyInput represents the output from a dependency task
type DependencyInput struct {
	// TaskID is the ID of the dependency task
	TaskID string
	// OutputDir is the directory containing the dependency's output files
	OutputDir string
	// Files are the relative paths to the files produced by the dependency
	Files []string
}

// Task represents a unit of work in the build graph
type Task interface {
	// ID returns a unique identifier for this task (uses hash for uniqueness)
	ID() string

	// Name returns a human-readable name for this task type
	Name() string

	// Directory returns the directory where this task was discovered
	Directory() string

	// TaskType returns the type of task (build or test)
	TaskType() TaskType

	// Hash returns a hash representing the task's configuration and inputs
	// This is used for caching and determining if a task needs to be re-executed
	Hash() string

	// Dependencies returns the list of tasks that must complete before this task can run
	Dependencies() []Task

	// Execute runs the task in the given working directory
	// dependencyInputs contains the outputs from all dependency tasks
	// It should return the relative paths to any files it creates
	Execute(ctx context.Context, workDir string, dependencyInputs []DependencyInput) TaskResult
}
