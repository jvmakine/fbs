package graph

import (
	"context"
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
	// ID returns a unique identifier for this task
	ID() string

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
