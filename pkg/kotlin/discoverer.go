package kotlin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
)

// KotlinDiscoverer discovers Kotlin compilation tasks from directories
type KotlinDiscoverer struct{}

// NewKotlinDiscoverer creates a new Kotlin discoverer
func NewKotlinDiscoverer() *KotlinDiscoverer {
	return &KotlinDiscoverer{}
}

// Name returns the name of this discoverer
func (d *KotlinDiscoverer) Name() string {
	return "KotlinDiscoverer"
}

// Discover finds Kotlin files in the given path and creates compilation tasks
func (d *KotlinDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *discoverer.BuildContext) (*discoverer.DiscoveryResult, error) {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist, return empty result
			return &discoverer.DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		}
		return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
	}
	
	var searchDir string
	if info.IsDir() {
		searchDir = path
	} else {
		// If it's a file, use its directory
		searchDir = filepath.Dir(path)
	}
	
	// Check if this is a source root directory (src/main/kotlin, src/test/kotlin, etc)
	isSourceRoot := d.isSourceRoot(searchDir)
	
	var kotlinFiles []string
	if isSourceRoot {
		// For source roots, recursively find all Kotlin files
		kotlinFiles, err = d.findKotlinFilesRecursive(searchDir)
		if err != nil {
			return &discoverer.DiscoveryResult{
				Tasks:  []graph.Task{},
				Errors: []error{err},
				Path:   path,
			}, nil
		}
	} else {
		// For non-source roots, only check immediate directory
		kotlinFiles, err = d.findKotlinFiles(searchDir)
		if err != nil {
			return &discoverer.DiscoveryResult{
				Tasks:  []graph.Task{},
				Errors: []error{err},
				Path:   path,
			}, nil
		}
		
		// Skip creating tasks for non-source-root directories that might be part of a larger source tree
		if len(kotlinFiles) > 0 && d.isPartOfSourceTree(searchDir) {
			// This directory has Kotlin files but appears to be part of a larger source tree
			// Let the source root handle compilation
			return &discoverer.DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		}
	}
	
	// If no Kotlin files found, return empty result
	if len(kotlinFiles) == 0 {
		return &discoverer.DiscoveryResult{
			Tasks: []graph.Task{},
			Path:  path,
		}, nil
	}
	
	// Create Kotlin compilation task
	task := NewKotlinCompile(searchDir, kotlinFiles)
	
	// Add potential dependencies as dependencies for this task
	// Filter to only include other Kotlin compilation tasks as dependencies
	for _, dep := range potentialDependencies {
		if kotlinDep, ok := dep.(*KotlinCompile); ok {
			task.AddDependency(kotlinDep)
		}
	}
	
	return &discoverer.DiscoveryResult{
		Tasks: []graph.Task{task},
		Path:  path,
	}, nil
}

// findKotlinFiles finds all .kt files in the given directory (non-recursive)
func (d *KotlinDiscoverer) findKotlinFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}
	
	var kotlinFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		if strings.HasSuffix(entry.Name(), ".kt") {
			kotlinFiles = append(kotlinFiles, entry.Name())
		}
	}
	
	return kotlinFiles, nil
}

// isSourceRoot checks if the given directory is a Kotlin source root
func (d *KotlinDiscoverer) isSourceRoot(dir string) bool {
	// Check if the directory ends with common Kotlin source root patterns
	return strings.HasSuffix(dir, "/src/main/kotlin") ||
		strings.HasSuffix(dir, "/src/test/kotlin") ||
		strings.HasSuffix(dir, "/src/dev/kotlin") ||
		strings.HasSuffix(dir, "/src/testFixtures/kotlin") ||
		strings.HasSuffix(dir, "/src/integrationTest/kotlin")
}

// isPartOfSourceTree checks if a directory appears to be part of a larger source tree
func (d *KotlinDiscoverer) isPartOfSourceTree(dir string) bool {
	// Check if any parent directory is a source root
	currentDir := dir
	for {
		parent := filepath.Dir(currentDir)
		if parent == currentDir || parent == "/" {
			break
		}
		
		if d.isSourceRoot(parent) {
			return true
		}
		
		currentDir = parent
	}
	return false
}

// findKotlinFilesRecursive finds all .kt files in the given directory tree (recursive)
func (d *KotlinDiscoverer) findKotlinFilesRecursive(rootDir string) ([]string, error) {
	var kotlinFiles []string
	
	err := filepath.Walk(rootDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		// Skip directories
		if info.IsDir() {
			return nil
		}
		
		// Check if it's a Kotlin file
		if strings.HasSuffix(info.Name(), ".kt") {
			// Get relative path from the root directory
			relPath, err := filepath.Rel(rootDir, path)
			if err != nil {
				return err
			}
			kotlinFiles = append(kotlinFiles, relPath)
		}
		
		return nil
	})
	
	if err != nil {
		return nil, fmt.Errorf("failed to walk directory %s: %w", rootDir, err)
	}
	
	return kotlinFiles, nil
}