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
	Version  bool     `short:"v" help:"Show version information"`
	Parallel int      `short:"j" help:"Number of parallel workers for task execution" default:"8"`
	Plan     PlanCmd  `cmd:"" help:"Plan and print the build graph"`
	Build    BuildCmd `cmd:"" help:"Execute build tasks in the specified directory"`
	Test     TestCmd  `cmd:"" help:"Execute test tasks in the specified directory"`
	Deps     DepsCmd  `cmd:"" help:"Execute dependency tasks in the specified directory"`
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

type DepsCmd struct {
	Directory string `arg:"" optional:"" help:"Directory to download dependencies for (defaults to current directory)"`
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
		err := runExecute(cli.Build.Directory, graph.TaskTypeBuild, cli.Parallel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "test <directory>", "test":
		err := runExecute(cli.Test.Directory, graph.TaskTypeTest, cli.Parallel)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	case "deps <directory>", "deps":
		err := runExecute(cli.Deps.Directory, graph.TaskTypeDeps, cli.Parallel)
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

	// Create structure discoverers
	structureDiscoverers := []discoverer.StructureDiscoverer{
		gradle.NewGradleStructureDiscoverer(),
	}

	// Create discoverers (excluding GradleDiscoverer since that's now handled by compilation root)
	discoverers := []discoverer.Discoverer{
		kotlin.NewKotlinDiscoverer(),
		kotlin.NewJunitDiscoverer(),
	}

	// Plan the build graph using structure-based approach
	ctx := context.Background()
	result, err := discoverer.PlanWithStructure(ctx, absDir, discoverers, structureDiscoverers)
	if err != nil {
		return fmt.Errorf("failed to plan build graph: %w", err)
	}

	// Print the results
	printStructurePlanResult(result, absDir)

	return nil
}

func runExecute(directory string, taskType graph.TaskType, parallelWorkers int) error {
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

	// Create structure discoverers
	structureDiscoverers := []discoverer.StructureDiscoverer{
		gradle.NewGradleStructureDiscoverer(),
	}

	// Create discoverers (excluding GradleDiscoverer since that's now handled by compilation root)
	discoverers := []discoverer.Discoverer{
		kotlin.NewKotlinDiscoverer(),
		kotlin.NewJunitDiscoverer(),
	}

	// Plan the build graph using structure-based approach
	ctx := context.Background()
	result, err := discoverer.PlanWithStructure(ctx, absDir, discoverers, structureDiscoverers)
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

	// Create a persistent cache directory for execution
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get user home directory: %w", err)
	}
	cacheDir := filepath.Join(homeDir, ".fbs", "cache")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Color constants
	const (
		green  = "\033[32m"
		orange = "\033[33m"
		red    = "\033[31m"
		reset  = "\033[0m"
	)

	// Get all tasks in execution order for display
	orderedTasks, err := executionGraph.TopologicalSort()
	if err != nil {
		return fmt.Errorf("failed to sort tasks: %w", err)
	}
	
	// Create task display tracking
	taskLines := make(map[string]int) // Map task ID to line number
	
	// Initialize all tasks as pending and display them
	for i, task := range orderedTasks {
		taskLines[task.ID()] = i
		
		// Get display path for task
		displayPath := ""
		if _, ok := task.(*gradle.ArtifactDownload); ok {
			// For artifact downloads, don't show the cache path
			displayPath = ""
		} else {
			relPath, err := filepath.Rel(absDir, task.Directory())
			if err != nil {
				relPath = task.Directory()
			}
			if relPath == "" {
				relPath = "."
			}
			displayPath = fmt.Sprintf(" (%s)", relPath)
		}
		
		fmt.Printf("  %s⏳%s %s%s\n", orange, reset, task.DisplayName(), displayPath)
	}
	
	// Progress callback to update task status in place
	progressCallback := func(task graph.Task, status string, finished bool, cached bool) {
		if !finished {
			return // Only update when task is finished
		}
		
		lineNum := taskLines[task.ID()]
		
		// Move cursor to the specific line and update it
		fmt.Printf("\033[%dA", len(orderedTasks)-lineNum) // Move up to the task's line
		fmt.Printf("\r\033[K") // Clear the line
		
		// Determine status symbol and color
		var statusSymbol, color string
		if status == "failed" {
			statusSymbol = "✗"
			color = red
		} else if cached {
			statusSymbol = "↻"  // Cached symbol
			color = "\033[36m"  // Cyan color for cached
		} else {
			statusSymbol = "✓"
			color = green
		}
		
		// Get display path for task
		displayPath := ""
		if _, ok := task.(*gradle.ArtifactDownload); ok {
			// For artifact downloads, don't show the cache path
			displayPath = ""
		} else {
			relPath, err := filepath.Rel(absDir, task.Directory())
			if err != nil {
				relPath = task.Directory()
			}
			if relPath == "" {
				relPath = "."
			}
			displayPath = fmt.Sprintf(" (%s)", relPath)
		}
		
		fmt.Printf("  %s%s%s %s%s\n", color, statusSymbol, reset, task.DisplayName(), displayPath)
		
		// Move cursor back to the bottom
		fmt.Printf("\033[%dB", len(orderedTasks)-lineNum-1)
	}

	// Execute the tasks with progress
	runner := graph.NewRunner(cacheDir)
	_, err = runner.ExecuteWithProgressParallel(ctx, executionGraph, progressCallback, parallelWorkers)
	
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	return nil
}

// filterTasksByTypeAndDirectory returns tasks of the specified type that are in or under the given directory
func filterTasksByTypeAndDirectory(tasks []graph.Task, taskType graph.TaskType, baseDir string) []graph.Task {
	var filtered []graph.Task
	for _, task := range tasks {
		if task.TaskType() == taskType {
			// For deps tasks, include all from the same compilation root regardless of directory
			// since they may be stored in cache directories outside the project
			if taskType == graph.TaskTypeDeps {
				filtered = append(filtered, task)
			} else {
				// For other tasks, check if task is in the specified directory or a subdirectory
				taskDir := task.Directory()
				relPath, err := filepath.Rel(baseDir, taskDir)
				if err == nil && !strings.HasPrefix(relPath, "..") {
					filtered = append(filtered, task)
				}
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

func printStructurePlanResult(result *discoverer.StructurePlanResult, baseDir string) {
	// Print compilation roots found
	fmt.Printf("Planning Directory: %s\n", result.RootDir)
	if len(result.CompilationRoots) > 0 {
		fmt.Println("Compilation Roots:")
		for i, root := range result.CompilationRoots {
			fmt.Printf("  %d. %s (%s)\n", i+1, root.GetRootDir(), root.GetType())
		}
		fmt.Println()
	}

	// Print tasks organized by compilation root
	tasks := result.Graph.GetTasks()
	if len(tasks) == 0 {
		fmt.Println("No tasks discovered.")
		return
	}

	// Group tasks by compilation root
	tasksByRoot := make(map[string][]graph.Task)
	for _, task := range tasks {
		if root, exists := result.TaskCompilationRoots[task.ID()]; exists {
			rootKey := root.GetRootDir()
			tasksByRoot[rootKey] = append(tasksByRoot[rootKey], task)
		} else {
			// Tasks without compilation root (shouldn't happen, but handle gracefully)
			tasksByRoot["unknown"] = append(tasksByRoot["unknown"], task)
		}
	}

	// Print tasks grouped by compilation root
	for rootDir, rootTasks := range tasksByRoot {
		if rootDir == "unknown" {
			fmt.Println("Tasks without compilation root:")
		} else {
			// Find the compilation root info
			var rootType string
			for _, root := range result.CompilationRoots {
				if root.GetRootDir() == rootDir {
					rootType = root.GetType()
					break
				}
			}
			
			relRootPath, err := filepath.Rel(baseDir, rootDir)
			if err != nil {
				relRootPath = rootDir
			}
			if relRootPath == "" {
				relRootPath = "."
			}
			
			fmt.Printf("Tasks from %s compilation root (%s):\n", rootType, relRootPath)
		}
		
		for _, task := range rootTasks {
			fmt.Print("  ")
			printTask(task, 0, baseDir)
		}
		fmt.Println()
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		fmt.Println("Errors:")
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
	magenta := "\033[35m"
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
	} else if task.TaskType() == graph.TaskTypeDeps {
		taskTypeColor = magenta
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
			} else if dep.TaskType() == graph.TaskTypeDeps {
				depTaskTypeColor = magenta
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