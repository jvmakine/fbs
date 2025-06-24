package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alecthomas/kong"

	"fbs/pkg/discoverer"
	"fbs/pkg/gradle"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

type CLI struct {
	Version bool      `short:"v" help:"Show version information"`
	Plan    PlanCmd   `cmd:"" help:"Plan and print the build graph"`
	Build   BuildCmd  `cmd:"" help:"Execute build tasks in the specified directory"`
	Test    TestCmd   `cmd:"" help:"Execute test tasks in the specified directory"`
}

type PlanCmd struct {
	Directory string `arg:"" optional:"" help:"Directory to plan (defaults to current directory)"`
}

type BuildCmd struct {
	Directory string `arg:"" optional:"" help:"Directory to build (defaults to current directory)"`
}

type TestCmd struct {
	Directory string `arg:"" optional:"" help:"Directory to test (defaults to current directory)"`
}

func main() {
	var cli CLI
	ctx := kong.Parse(&cli)

	switch ctx.Command() {
	case "plan <directory>", "plan":
		err := runPlan(cli.Plan)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "build <directory>", "build":
		err := runExecute(cli.Build.Directory, graph.TaskTypeBuild)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "test <directory>", "test":
		err := runExecute(cli.Test.Directory, graph.TaskTypeTest)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		if cli.Version {
			fmt.Println("fbs version 1.0.0")
			return
		}
		fmt.Println("Hello, World!")
	}
}

func runPlan(cmd PlanCmd) error {
	// Determine the directory to plan
	planDir := cmd.Directory
	if planDir == "" {
		var err error
		planDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Convert to absolute path
	absDir, err := filepath.Abs(planDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Change to the target directory for planning
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	err = os.Chdir(absDir)
	if err != nil {
		return fmt.Errorf("failed to change to directory %s: %w", absDir, err)
	}

	// Create discoverers
	discoverers := []discoverer.Discoverer{
		kotlin.NewKotlinDiscoverer(),
		kotlin.NewJunitDiscoverer(),
		gradle.NewGradleDiscoverer(),
	}

	// Plan the build graph
	ctx := context.Background()
	result, err := discoverer.Plan(ctx, discoverers)
	if err != nil {
		return fmt.Errorf("failed to plan build graph: %w", err)
	}

	// Print the results
	printPlanResult(result, absDir)

	return nil
}

func runExecute(directory string, taskType graph.TaskType) error {
	// Determine the directory to execute in
	execDir := directory
	if execDir == "" {
		var err error
		execDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Convert to absolute path
	absDir, err := filepath.Abs(execDir)
	if err != nil {
		return fmt.Errorf("failed to get absolute path: %w", err)
	}

	// Change to the target directory for planning
	originalDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}
	defer os.Chdir(originalDir)

	err = os.Chdir(absDir)
	if err != nil {
		return fmt.Errorf("failed to change to directory %s: %w", absDir, err)
	}

	// Create discoverers
	discoverers := []discoverer.Discoverer{
		kotlin.NewKotlinDiscoverer(),
		kotlin.NewJunitDiscoverer(),
		gradle.NewGradleDiscoverer(),
	}

	// Plan the build graph
	ctx := context.Background()
	result, err := discoverer.Plan(ctx, discoverers)
	if err != nil {
		return fmt.Errorf("failed to plan build graph: %w", err)
	}

	// Filter tasks by type and directory
	filteredTasks := filterTasksByTypeAndDirectory(result.Graph.GetTasks(), taskType, absDir)
	
	if len(filteredTasks) == 0 {
		fmt.Printf("No %s tasks found in directory %s\n", taskType, absDir)
		return nil
	}

	// Create a new graph with only filtered tasks and their dependencies
	executionGraph := createExecutionGraph(filteredTasks)

	// Create a temporary directory for execution
	tempDir, err := os.MkdirTemp("", "fbs-execution")
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Execute the tasks
	runner := graph.NewRunner(tempDir)
	execResults, err := runner.Execute(ctx, executionGraph)
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Print execution results
	fmt.Printf("Executed %d %s tasks:\n", len(execResults), taskType)
	for _, result := range execResults {
		status := "✓"
		if result.Result.Error != nil {
			status = "✗"
		}
		
		relPath, err := filepath.Rel(absDir, result.Task.Directory())
		if err != nil {
			relPath = result.Task.Directory()
		}
		if relPath == "" {
			relPath = "."
		}

		fmt.Printf("  %s %s (%s)\n", status, result.Task.Name(), relPath)
		if result.Result.Error != nil {
			fmt.Printf("    Error: %v\n", result.Result.Error)
		}
	}

	return nil
}

// filterTasksByTypeAndDirectory returns tasks of the specified type that are in or under the given directory
func filterTasksByTypeAndDirectory(tasks []graph.Task, taskType graph.TaskType, baseDir string) []graph.Task {
	var filtered []graph.Task
	for _, task := range tasks {
		if task.TaskType() == taskType {
			// Check if task is in the specified directory or a subdirectory
			taskDir := task.Directory()
			relPath, err := filepath.Rel(baseDir, taskDir)
			if err == nil && !strings.HasPrefix(relPath, "..") {
				filtered = append(filtered, task)
			}
		}
	}
	return filtered
}

// createExecutionGraph creates a new graph containing the filtered tasks and all their dependencies
func createExecutionGraph(filteredTasks []graph.Task) *graph.Graph {
	executionGraph := graph.NewGraph()
	visited := make(map[string]bool)

	var addTaskWithDependencies func(task graph.Task)
	addTaskWithDependencies = func(task graph.Task) {
		if visited[task.ID()] {
			return
		}
		visited[task.ID()] = true

		// Add all dependencies first
		for _, dep := range task.Dependencies() {
			addTaskWithDependencies(dep)
		}

		// Add the task itself
		executionGraph.AddTask(task)
	}

	// Add all filtered tasks and their dependencies
	for _, task := range filteredTasks {
		addTaskWithDependencies(task)
	}

	return executionGraph
}

func printPlanResult(result *discoverer.PlanResult, baseDir string) {
	// Print tasks
	tasks := result.Graph.GetTasks()
	if len(tasks) == 0 {
		fmt.Println("No tasks discovered.")
		return
	}

	for _, task := range tasks {
		printTask(task, 0, baseDir)
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		for i, err := range result.Errors {
			fmt.Printf("%d. %v\n", i+1, err)
		}
	}
}

func printTask(task graph.Task, indent int, baseDir string) {
	indentStr := ""
	for i := 0; i < indent; i++ {
		indentStr += "  "
	}

	// Colors
	green := "\033[32m"
	gray := "\033[90m"
	blue := "\033[34m"
	yellow := "\033[33m"
	cyan := "\033[36m"
	reset := "\033[0m"

	// Convert absolute path to relative path
	relPath, err := filepath.Rel(baseDir, task.Directory())
	if err != nil {
		relPath = task.Directory() // fallback to absolute if conversion fails
	}
	
	// Use "." for current directory
	if relPath == "" {
		relPath = "."
	}

	// Get task type and color
	taskTypeColor := yellow
	if task.TaskType() == graph.TaskTypeTest {
		taskTypeColor = cyan
	}

	fmt.Printf("%s- %s%s%s %s[%s]%s %s(%s)%s %s%s%s\n", 
		indentStr, 
		green, task.Name(), reset,
		taskTypeColor, task.TaskType(), reset,
		blue, relPath, reset,
		gray, task.Hash()[:8], reset)
	
	// Print dependencies with path and hash
	deps := task.Dependencies()
	if len(deps) > 0 {
		for _, dep := range deps {
			depRelPath, err := filepath.Rel(baseDir, dep.Directory())
			if err != nil {
				depRelPath = dep.Directory()
			}
			if depRelPath == "" {
				depRelPath = "."
			}
			
			// Get dependency task type and color
			depTaskTypeColor := yellow
			if dep.TaskType() == graph.TaskTypeTest {
				depTaskTypeColor = cyan
			}

			fmt.Printf("%s    -> %s%s%s %s[%s]%s %s(%s)%s %s%s%s\n", 
				indentStr, 
				green, dep.Name(), reset,
				depTaskTypeColor, dep.TaskType(), reset,
				blue, depRelPath, reset,
				gray, dep.Hash()[:8], reset)
		}
	}
}