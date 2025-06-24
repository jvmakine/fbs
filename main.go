package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/alecthomas/kong"

	"fbs/pkg/discoverer"
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
		// Add more discoverers here as they are implemented
	}

	// Plan the build graph
	ctx := context.Background()
	result, err := discoverer.Plan(ctx, discoverers)
	if err != nil {
		return fmt.Errorf("failed to plan build graph: %w", err)
	}

	// Print the results
	printPlanResult(result)

	return nil
}

func printPlanResult(result *discoverer.PlanResult) {
	fmt.Printf("Build Graph Planning Results\n")
	fmt.Printf("============================\n\n")
	
	fmt.Printf("Git Root: %s\n", result.RootDir)
	fmt.Printf("Scanned Directories: %d\n", len(result.ScannedDirs))
	fmt.Printf("Discovered Tasks: %d\n", len(result.Graph.GetTasks()))
	
	if len(result.Errors) > 0 {
		fmt.Printf("Errors: %d\n", len(result.Errors))
	}
	
	fmt.Println()

	// Print tasks
	tasks := result.Graph.GetTasks()
	if len(tasks) == 0 {
		fmt.Println("No tasks discovered.")
		return
	}

	fmt.Println("Tasks:")
	fmt.Println("------")
	for _, task := range tasks {
		printTask(task, 0)
	}

	// Print errors if any
	if len(result.Errors) > 0 {
		fmt.Println("\nErrors:")
		fmt.Println("-------")
		for i, err := range result.Errors {
			fmt.Printf("%d. %v\n", i+1, err)
		}
	}
}

func printTask(task graph.Task, indent int) {
	indentStr := ""
	for i := 0; i < indent; i++ {
		indentStr += "  "
	}

	fmt.Printf("%s- %s (hash: %s)\n", indentStr, task.ID(), task.Hash()[:8])
	
	// Print dependencies
	deps := task.Dependencies()
	fmt.Printf("%s  Dependencies: %d\n", indentStr, len(deps))
	if len(deps) > 0 {
		for _, dep := range deps {
			fmt.Printf("%s    -> %s\n", indentStr, dep.ID())
		}
	}
}