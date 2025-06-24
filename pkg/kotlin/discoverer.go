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
func (d *KotlinDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task) (*discoverer.DiscoveryResult, error) {
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
	
	// Find Kotlin files in the root of the directory (not recursive)
	kotlinFiles, err := d.findKotlinFiles(searchDir)
	if err != nil {
		return &discoverer.DiscoveryResult{
			Tasks:  []graph.Task{},
			Errors: []error{err},
			Path:   path,
		}, nil
	}
	
	// If no Kotlin files found, return empty result
	if len(kotlinFiles) == 0 {
		return &discoverer.DiscoveryResult{
			Tasks: []graph.Task{},
			Path:  path,
		}, nil
	}
	
	// Create Kotlin compilation task
	taskID := fmt.Sprintf("kotlin-compile-%s", filepath.Base(searchDir))
	task := NewKotlinCompile(taskID, searchDir, kotlinFiles)
	
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