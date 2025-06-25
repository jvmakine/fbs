package discoverer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/config"
	"fbs/pkg/graph"
)

// StructurePlanResult represents the result of structure-based planning
type StructurePlanResult struct {
	// Graph contains the discovered tasks
	Graph *graph.Graph
	// Errors contains any errors encountered during discovery
	Errors []error
	// RootDir is the directory that was planned
	RootDir string
	// TaskCompilationRoots maps task IDs to their compilation roots
	TaskCompilationRoots map[string]CompilationRoot
	// CompilationRoots contains all compilation roots found during planning
	CompilationRoots []CompilationRoot
}

// PlanWithStructure discovers build tasks using structure-based discovery
// Given a directory, it discovers all tasks from subdirectories, finding their
// compilation roots and organizing them accordingly
func PlanWithStructure(ctx context.Context, dir string, discoverers []Discoverer, structureDiscoverers []StructureDiscoverer) (*StructurePlanResult, error) {
	// Clean the directory path
	dir = filepath.Clean(dir)
	
	// Load configuration from directory hierarchy
	configuration, err := config.LoadConfiguration(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to load configuration: %w", err)
	}
	
	// Create new graph
	buildGraph := graph.NewGraph()
	var allErrors []error
	
	// Maps to track compilation roots and task associations
	taskCompilationRoots := make(map[string]CompilationRoot)
	compilationRootMap := make(map[string]CompilationRoot) // Map by root directory
	
	// First, collect all valid directories under the specified directory
	var validDirs []string
	err = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("error accessing path %s: %w", path, err))
			return nil // Continue walking
		}
		
		// Skip non-directories
		if !info.IsDir() {
			return nil
		}
		
		// Skip hidden directories (except the root if it's hidden)
		if strings.HasPrefix(info.Name(), ".") && path != dir {
			return filepath.SkipDir
		}
		
		// Skip common build/output directories
		if isSkippableDir(info.Name()) {
			return filepath.SkipDir
		}
		
		validDirs = append(validDirs, path)
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to collect directories: %w", err)
	}
	
	// Sort directories by depth (deepest first) for bottom-up processing
	sortDirectoriesByDepth(validDirs)
	
	// Map to store tasks discovered in each directory
	tasksByDir := make(map[string][]graph.Task)
	
	// Process directories in bottom-up order
	for _, dirPath := range validDirs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		// Find compilation root for this directory
		compilationRoot, err := findCompilationRoot(ctx, dirPath, structureDiscoverers)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to find compilation root for %s: %w", dirPath, err))
			continue
		}
		
		if compilationRoot == nil {
			// No compilation root found, skip this directory
			continue
		}
		
		// Store the compilation root
		rootDir := compilationRoot.GetRootDir()
		compilationRootMap[rootDir] = compilationRoot
		
		// Get build context from the compilation root
		buildContext := compilationRoot.GetBuildContext(dirPath)
		
		// Add configuration to the build context
		buildContext.Set(configuration)
		
		// Collect potential dependencies from subdirectories
		var potentialDeps []graph.Task
		for subDir, tasks := range tasksByDir {
			// Check if subDir is a subdirectory of current dirPath
			if isSubdirectory(dirPath, subDir) {
				potentialDeps = append(potentialDeps, tasks...)
			}
		}
		
		// Discover tasks in this directory
		var dirTasks []graph.Task
		for _, disc := range discoverers {
			result, err := disc.Discover(ctx, dirPath, potentialDeps, buildContext)
			if err != nil {
				allErrors = append(allErrors, fmt.Errorf("discoverer %s failed on %s: %w", disc.Name(), dirPath, err))
				continue
			}
			
			// Add discovered tasks to our collection
			dirTasks = append(dirTasks, result.Tasks...)
			
			// Collect any discovery errors
			allErrors = append(allErrors, result.Errors...)
		}
		
		// Let the compilation root process task dependencies
		dirTasks = compilationRoot.GetTaskDependencies(dirPath, dirTasks, buildContext)
		
		// Add tasks to the graph and track their compilation roots
		for _, task := range dirTasks {
			if err := buildGraph.AddTask(task); err != nil {
				// If task already exists, that's okay - just skip it
				if !strings.Contains(err.Error(), "already exists") {
					allErrors = append(allErrors, fmt.Errorf("failed to add task %s: %w", task.ID(), err))
				}
			} else {
				// Track which compilation root this task belongs to
				taskCompilationRoots[task.ID()] = compilationRoot
			}
		}
		
		// Store tasks found in this directory
		if len(dirTasks) > 0 {
			tasksByDir[dirPath] = dirTasks
		}
	}
	
	// Convert compilation root map to slice
	var compilationRoots []CompilationRoot
	for _, root := range compilationRootMap {
		compilationRoots = append(compilationRoots, root)
	}
	
	// Resolve inter-module project dependencies
	for _, root := range compilationRoots {
		err = root.ResolveProjectDependencies(buildGraph, compilationRoots)
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("failed to resolve project dependencies for %s: %w", root.GetRootDir(), err))
		}
	}
	
	return &StructurePlanResult{
		Graph:                buildGraph,
		Errors:               allErrors,
		RootDir:              dir,
		TaskCompilationRoots: taskCompilationRoots,
		CompilationRoots:     compilationRoots,
	}, nil
}

// findCompilationRoot traverses upwards from the given directory to find a compilation root
func findCompilationRoot(ctx context.Context, startDir string, structureDiscoverers []StructureDiscoverer) (CompilationRoot, error) {
	currentDir := startDir
	
	for {
		// Check if current directory is a compilation root
		for _, structureDisc := range structureDiscoverers {
			root, err := structureDisc.IsCompilationRoot(ctx, currentDir)
			if err != nil {
				return nil, fmt.Errorf("structure discoverer %s failed on %s: %w", structureDisc.Name(), currentDir, err)
			}
			
			if root != nil {
				return root, nil
			}
		}
		
		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached the filesystem root
			break
		}
		currentDir = parentDir
	}
	
	return nil, nil // No compilation root found
}

