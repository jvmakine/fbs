package discoverer

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fbs/pkg/graph"
)

// MockPlanDiscoverer for testing Plan functionality
type MockPlanDiscoverer struct {
	name         string
	discoverFunc func(ctx context.Context, path string, potentialDependencies []graph.Task) (*DiscoveryResult, error)
}

func NewMockPlanDiscoverer(name string, discoverFunc func(context.Context, string, []graph.Task) (*DiscoveryResult, error)) *MockPlanDiscoverer {
	return &MockPlanDiscoverer{
		name:         name,
		discoverFunc: discoverFunc,
	}
}

func (m *MockPlanDiscoverer) Name() string {
	return m.name
}

func (m *MockPlanDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task) (*DiscoveryResult, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, path, potentialDependencies)
	}
	return &DiscoveryResult{
		Tasks: []graph.Task{},
		Path:  path,
	}, nil
}

// MockPlanTask for testing
type MockPlanTask struct {
	id   string
	hash string
}

func (m *MockPlanTask) ID() string {
	return m.id
}

func (m *MockPlanTask) Hash() string {
	return m.hash
}

func (m *MockPlanTask) Dependencies() []graph.Task {
	return nil
}

func (m *MockPlanTask) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	return graph.TaskResult{Files: []string{m.id + ".txt"}}
}

func TestFindGitRoot(t *testing.T) {
	// This test assumes we're running in a git repository
	rootDir, err := findGitRoot()
	if err != nil {
		t.Skipf("Skipping test - not in a git repository: %v", err)
	}
	
	if rootDir == "" {
		t.Error("Git root should not be empty")
	}
	
	// Verify .git directory exists
	gitDir := filepath.Join(rootDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		t.Errorf("Expected .git directory to exist at %s", gitDir)
	}
}

func TestIsSkippableDir(t *testing.T) {
	skippableDirs := []string{
		"node_modules",
		"target",
		"build",
		"dist",
		".idea",
		"__pycache__",
	}
	
	for _, dir := range skippableDirs {
		if !isSkippableDir(dir) {
			t.Errorf("Expected %s to be skippable", dir)
		}
	}
	
	normalDirs := []string{
		"src",
		"lib",
		"pkg",
		"cmd",
		"examples",
	}
	
	for _, dir := range normalDirs {
		if isSkippableDir(dir) {
			t.Errorf("Expected %s not to be skippable", dir)
		}
	}
}

func TestPlan_MockDiscoverers(t *testing.T) {
	// Create a temporary git repository for testing
	tempDir, err := os.MkdirTemp("", "plan_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)
	
	// Initialize as git repo
	gitDir := filepath.Join(tempDir, ".git")
	err = os.MkdirAll(gitDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create .git dir: %v", err)
	}
	
	// Create some project structure
	srcDir := filepath.Join(tempDir, "src")
	err = os.MkdirAll(srcDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create src dir: %v", err)
	}
	
	libDir := filepath.Join(tempDir, "lib")
	err = os.MkdirAll(libDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create lib dir: %v", err)
	}
	
	// Create a directory that should be skipped
	nodeModulesDir := filepath.Join(tempDir, "node_modules")
	err = os.MkdirAll(nodeModulesDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create node_modules dir: %v", err)
	}
	
	// Save current directory and change to temp dir
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(originalDir)
	
	err = os.Chdir(tempDir)
	if err != nil {
		t.Fatalf("Failed to change to temp dir: %v", err)
	}
	
	// Create mock discoverers
	taskCount := 0
	kotlinDiscoverer := NewMockPlanDiscoverer("KotlinTest",
		func(ctx context.Context, path string, potentialDependencies []graph.Task) (*DiscoveryResult, error) {
			// Only create tasks for src and lib directories
			if filepath.Base(path) == "src" || filepath.Base(path) == "lib" {
				taskCount++
				return &DiscoveryResult{
					Tasks: []graph.Task{
						&MockPlanTask{
							id:   fmt.Sprintf("kotlin-compile-%s-%d", filepath.Base(path), taskCount),
							hash: fmt.Sprintf("hash-%d", taskCount),
						},
					},
					Path: path,
				}, nil
			}
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})
	
	goDiscoverer := NewMockPlanDiscoverer("GoTest",
		func(ctx context.Context, path string, potentialDependencies []graph.Task) (*DiscoveryResult, error) {
			// Only create task for root directory
			if path == tempDir {
				taskCount++
				return &DiscoveryResult{
					Tasks: []graph.Task{
						&MockPlanTask{
							id:   fmt.Sprintf("go-build-%d", taskCount),
							hash: fmt.Sprintf("hash-%d", taskCount),
						},
					},
					Path: path,
				}, nil
			}
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})
	
	discoverers := []Discoverer{kotlinDiscoverer, goDiscoverer}
	
	ctx := context.Background()
	result, err := Plan(ctx, discoverers)
	if err != nil {
		t.Fatalf("Plan failed: %v", err)
	}
	
	if result.RootDir != tempDir {
		t.Errorf("Expected root dir %s, got %s", tempDir, result.RootDir)
	}
	
	// Should have found at least 3 tasks (2 from kotlin + 1 from go)
	tasks := result.Graph.GetTasks()
	if len(tasks) < 3 {
		t.Errorf("Expected at least 3 tasks, got %d", len(tasks))
	}
	
	// Verify that node_modules was not scanned
	foundNodeModules := false
	for _, scannedDir := range result.ScannedDirs {
		if filepath.Base(scannedDir) == "node_modules" {
			foundNodeModules = true
			break
		}
	}
	if foundNodeModules {
		t.Error("node_modules directory should have been skipped")
	}
	
	// Verify that src and lib directories were scanned
	foundSrc := false
	foundLib := false
	for _, scannedDir := range result.ScannedDirs {
		base := filepath.Base(scannedDir)
		if base == "src" {
			foundSrc = true
		}
		if base == "lib" {
			foundLib = true
		}
	}
	if !foundSrc {
		t.Error("src directory should have been scanned")
	}
	if !foundLib {
		t.Error("lib directory should have been scanned")
	}
}

func TestPlan_ContextCancellation(t *testing.T) {
	// Save current directory
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get current dir: %v", err)
	}
	defer os.Chdir(originalDir)
	
	// Create a discoverer that never returns tasks
	slowDiscoverer := NewMockPlanDiscoverer("Slow",
		func(ctx context.Context, path string, potentialDependencies []graph.Task) (*DiscoveryResult, error) {
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})
	
	discoverers := []Discoverer{slowDiscoverer}
	
	// Create a cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	
	_, err = Plan(ctx, discoverers)
	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
	
	// Check if the error contains "context canceled"
	if !strings.Contains(err.Error(), "context canceled") {
		t.Errorf("Expected error to contain 'context canceled', got: %v", err)
	}
}

func TestPlan_EmptyDiscoverers(t *testing.T) {
	// Test with no discoverers
	ctx := context.Background()
	result, err := Plan(ctx, []Discoverer{})
	if err != nil {
		t.Fatalf("Plan with empty discoverers failed: %v", err)
	}
	
	tasks := result.Graph.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks with no discoverers, got %d", len(tasks))
	}
	
	if result.RootDir == "" {
		t.Error("Root directory should still be found even with no discoverers")
	}
}