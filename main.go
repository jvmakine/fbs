package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"fbs/pkg/discoverer"
	"fbs/pkg/gradle"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

type CLI struct {
	Version bool      `short:"v" help:"Show version information"`
	Plan    PlanCmd   `cmd:"" help:"Plan and print the build graph"`
}

type PlanCmd struct {
	Directory string `arg:"" optional:"" help:"Directory to plan (defaults to current directory)"`
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

	fmt.Printf("%s- %s%s%s %s(%s)%s %s%s%s\n", 
		indentStr, 
		green, task.Name(), reset,
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
			
			fmt.Printf("%s    -> %s%s%s %s(%s)%s %s%s%s\n", 
				indentStr, 
				green, dep.Name(), reset,
				blue, depRelPath, reset,
				gray, dep.Hash()[:8], reset)
		}
	}
}