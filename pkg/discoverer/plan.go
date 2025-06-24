package discoverer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/graph"
)

// PlanResult represents the result of planning a build graph
type PlanResult struct {
	// Graph contains the discovered tasks
	Graph *graph.Graph
	// Errors contains any errors encountered during discovery
	Errors []error
	// RootDir is the git root directory that was scanned
	RootDir string
	// ScannedDirs is the list of directories that were scanned
	ScannedDirs []string
}

// Plan discovers all build tasks in a git repository by traversing all directories
// and running the provided discoverers on each directory. It processes directories
// in bottom-up order so that subdirectory tasks can be passed as potential dependencies
// to parent directory discoverers. Context discoverers are run first to populate BuildContext.
func Plan(ctx context.Context, discoverers []Discoverer, contextDiscoverers []ContextDiscoverer) (*PlanResult, error) {
	// Find git root directory
	rootDir, err := findGitRoot()
	if err != nil {
		return nil, fmt.Errorf("failed to find git root: %w", err)
	}
	
	// Create new graph
	buildGraph := graph.NewGraph()
	
	var allErrors []error
	var scannedDirs []string
	
	// First, collect all valid directories in the tree
	var validDirs []string
	err = filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			allErrors = append(allErrors, fmt.Errorf("error accessing path %s: %w", path, err))
			return nil // Continue walking
		}
		
		// Skip non-directories
		if !info.IsDir() {
			return nil
		}
		
		// Skip .git directory and other hidden directories
		if strings.HasPrefix(info.Name(), ".") && path != rootDir {
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
	
	// Map to store build context for each directory
	contextsByDir := make(map[string]*BuildContext)
	
	// Create root build context
	rootContext := NewBuildContext()
	contextsByDir[rootDir] = rootContext
	
	// Process directories in bottom-up order
	for _, dirPath := range validDirs {
		// Check context cancellation
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}
		
		scannedDirs = append(scannedDirs, dirPath)
		
		// Get or create build context for this directory
		buildContext := getBuildContextForDirectory(dirPath, contextsByDir)
		
		// Run context discoverers to populate build context for this directory
		for _, contextDisc := range contextDiscoverers {
			if err := contextDisc.DiscoverContext(ctx, dirPath, buildContext); err != nil {
				allErrors = append(allErrors, fmt.Errorf("context discoverer %s failed on %s: %w", contextDisc.Name(), dirPath, err))
			}
		}
		
		// Store the updated context for this directory
		contextsByDir[dirPath] = buildContext
		
		// Collect potential dependencies from subdirectories
		var potentialDeps []graph.Task
		for subDir, tasks := range tasksByDir {
			// Check if subDir is a subdirectory of current dirPath
			if isSubdirectory(dirPath, subDir) {
				potentialDeps = append(potentialDeps, tasks...)
			}
		}
		
		// Run all discoverers on this directory with potential dependencies and build context
		var dirTasks []graph.Task
		for _, disc := range discoverers {
			result, err := disc.Discover(ctx, dirPath, potentialDeps, buildContext)
			if err != nil {
				allErrors = append(allErrors, fmt.Errorf("discoverer %s failed on %s: %w", disc.Name(), dirPath, err))
				continue
			}
			
			// Add discovered tasks to graph
			for _, task := range result.Tasks {
				if err := buildGraph.AddTask(task); err != nil {
					// If task already exists, that's okay - just skip it
					if !strings.Contains(err.Error(), "already exists") {
						allErrors = append(allErrors, fmt.Errorf("failed to add task %s: %w", task.ID(), err))
					}
				} else {
					dirTasks = append(dirTasks, task)
				}
			}
			
			// Collect any discovery errors
			allErrors = append(allErrors, result.Errors...)
		}
		
		// Store tasks found in this directory
		if len(dirTasks) > 0 {
			tasksByDir[dirPath] = dirTasks
		}
	}
	
	return &PlanResult{
		Graph:       buildGraph,
		Errors:      allErrors,
		RootDir:     rootDir,
		ScannedDirs: scannedDirs,
	}, nil
}

// sortDirectoriesByDepth sorts directories by depth (deepest first)
func sortDirectoriesByDepth(dirs []string) {
	// Simple bubble sort by path depth (deeper paths have more separators)
	for i := 0; i < len(dirs); i++ {
		for j := i + 1; j < len(dirs); j++ {
			depthI := strings.Count(dirs[i], string(filepath.Separator))
			depthJ := strings.Count(dirs[j], string(filepath.Separator))
			if depthI < depthJ {
				dirs[i], dirs[j] = dirs[j], dirs[i]
			}
		}
	}
}

// isSubdirectory checks if subPath is a subdirectory of parentPath
func isSubdirectory(parentPath, subPath string) bool {
	// Clean paths to handle . and .. elements
	parentPath = filepath.Clean(parentPath)
	subPath = filepath.Clean(subPath)
	
	// subPath must be longer than parentPath to be a subdirectory
	if len(subPath) <= len(parentPath) {
		return false
	}
	
	// Check if subPath starts with parentPath followed by a separator
	return strings.HasPrefix(subPath, parentPath+string(filepath.Separator))
}

// findGitRoot finds the root directory of the git repository
func findGitRoot() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}
	
	// Walk up the directory tree looking for .git directory
	dir := currentDir
	for {
		gitDir := filepath.Join(dir, ".git")
		if info, err := os.Stat(gitDir); err == nil {
			if info.IsDir() {
				return dir, nil
			}
			// .git might be a file in case of worktrees, check if it contains gitdir
			if content, err := os.ReadFile(gitDir); err == nil {
				if strings.HasPrefix(string(content), "gitdir:") {
					return dir, nil
				}
			}
		}
		
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached filesystem root
			break
		}
		dir = parent
	}
	
	return "", fmt.Errorf("not in a git repository")
}

// isSkippableDir returns true if the directory should be skipped during discovery
func isSkippableDir(dirName string) bool {
	skipDirs := []string{
		"node_modules",
		"target",
		"build",
		"dist",
		"out",
		"bin",
		"obj",
		"Debug",
		"Release",
		"__pycache__",
		".gradle",
		".idea",
		".vscode",
		".vs",
		"vendor",
		"deps",
		"_build",
		".tox",
		".pytest_cache",
		".coverage",
		"htmlcov",
	}
	
	for _, skip := range skipDirs {
		if dirName == skip {
			return true
		}
	}
	
	return false
}

// getBuildContextForDirectory gets or creates a build context for the given directory
// It copies context from the parent directory if available
func getBuildContextForDirectory(dirPath string, contextsByDir map[string]*BuildContext) *BuildContext {
	// If we already have context for this directory, return it
	if context, exists := contextsByDir[dirPath]; exists {
		return context
	}
	
	// Find the parent directory that has context
	parentDir := filepath.Dir(dirPath)
	for parentDir != dirPath { // Stop when we reach the root
		if parentContext, exists := contextsByDir[parentDir]; exists {
			// Copy parent context for this directory
			return parentContext.Copy()
		}
		nextParent := filepath.Dir(parentDir)
		if nextParent == parentDir {
			// Reached filesystem root
			break
		}
		parentDir = nextParent
	}
	
	// No parent context found, create new empty context
	return NewBuildContext()
}