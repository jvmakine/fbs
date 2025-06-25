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

// JunitDiscoverer discovers JUnit test tasks from Kotlin test files
type JunitDiscoverer struct{}

// NewJunitDiscoverer creates a new JUnit test discoverer
func NewJunitDiscoverer() *JunitDiscoverer {
	return &JunitDiscoverer{}
}

// Name returns the name of this discoverer
func (d *JunitDiscoverer) Name() string {
	return "JunitDiscoverer"
}

// Discover finds Kotlin test files and creates JUnit test tasks
func (d *JunitDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *discoverer.BuildContext) (*discoverer.DiscoveryResult, error) {
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
	
	// Find Kotlin test files in the root of the directory (not recursive)
	testFiles, err := d.findKotlinTestFiles(searchDir)
	if err != nil {
		return &discoverer.DiscoveryResult{
			Tasks:  []graph.Task{},
			Errors: []error{err},
			Path:   path,
		}, nil
	}
	
	// If no test files found, return empty result
	if len(testFiles) == 0 {
		return &discoverer.DiscoveryResult{
			Tasks: []graph.Task{},
			Path:  path,
		}, nil
	}
	
	// Create JUnit test tasks for each test file
	var tasks []graph.Task
	for _, testFile := range testFiles {
		className := d.extractClassName(testFile)
		task := NewJunitTest(testFile, searchDir, className)
		
		// Add potential dependencies (typically KotlinCompile tasks)
		for _, dep := range potentialDependencies {
			if _, ok := dep.(*KotlinCompile); ok {
				task.AddDependency(dep)
			}
		}
		
		tasks = append(tasks, task)
	}
	
	return &discoverer.DiscoveryResult{
		Tasks: tasks,
		Path:  path,
	}, nil
}

// findKotlinTestFiles finds all .kt files that end with Test.kt and are under src/test (non-recursive)
func (d *JunitDiscoverer) findKotlinTestFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", dir, err)
	}
	
	var testFiles []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		
		// Check if file ends with Test.kt and is under src/test
		if d.isTestFile(entry.Name(), dir) {
			testFiles = append(testFiles, entry.Name())
		}
	}
	
	return testFiles, nil
}

// isTestFile checks if a Kotlin file is a test file based on naming convention and path
func (d *JunitDiscoverer) isTestFile(fileName, dir string) bool {
	// Check if file ends with Test.kt
	if !strings.HasSuffix(fileName, "Test.kt") {
		return false
	}
	
	// Check if directory path contains src/test
	return strings.Contains(dir, "src/test")
}

// extractClassName extracts the class name from a Kotlin file name
func (d *JunitDiscoverer) extractClassName(fileName string) string {
	// Remove .kt extension and return the base name
	baseName := strings.TrimSuffix(fileName, ".kt")
	return baseName
}