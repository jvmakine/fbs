package discoverer

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fbs/pkg/graph"
)

// MockStructureDiscoverer for testing
type MockStructureDiscoverer struct {
	name        string
	checkFunc   func(string) CompilationRoot
}

func (m *MockStructureDiscoverer) Name() string {
	return m.name
}

func (m *MockStructureDiscoverer) IsCompilationRoot(ctx context.Context, dir string) (CompilationRoot, error) {
	return m.checkFunc(dir), nil
}

// MockCompilationRoot for testing
type MockCompilationRoot struct {
	rootDir     string
	rootType    string
	buildContext *BuildContext
}

func (m *MockCompilationRoot) GetRootDir() string {
	return m.rootDir
}

func (m *MockCompilationRoot) GetType() string {
	return m.rootType
}

func (m *MockCompilationRoot) GetBuildContext(dir string) *BuildContext {
	if m.buildContext != nil {
		return m.buildContext
	}
	return NewBuildContext()
}

func (m *MockCompilationRoot) GetTaskDependencies(dir string, tasks []graph.Task) []graph.Task {
	return tasks // Return tasks unchanged for simple testing
}

func TestPlanWithStructure_FindsCompilationRoot(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "structure_plan_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create nested directory structure
	subDir := filepath.Join(tempDir, "src", "main", "kotlin")
	err = os.MkdirAll(subDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	// Create a mock structure discoverer that recognizes tempDir as a compilation root
	structureDisc := &MockStructureDiscoverer{
		name: "MockStructure",
		checkFunc: func(dir string) CompilationRoot {
			resolvedDir, _ := filepath.EvalSymlinks(dir)
			resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
			if resolvedDir == resolvedTempDir {
				return &MockCompilationRoot{
					rootDir:  dir,
					rootType: "mock",
				}
			}
			return nil
		},
	}

	// Create a mock discoverer that creates a task
	taskCount := 0
	discoverer := NewMockPlanDiscoverer("MockDiscoverer",
		func(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *BuildContext) (*DiscoveryResult, error) {
			taskCount++
			return &DiscoveryResult{
				Tasks: []graph.Task{
					&MockPlanTask{
						id:        "mock-task-1",
						name:      "mock-task",
						directory: path,
						hash:      "hash-1",
					},
				},
				Path: path,
			}, nil
		})

	// Test planning from the nested subdirectory
	ctx := context.Background()
	result, err := PlanWithStructure(ctx, subDir, []Discoverer{discoverer}, []StructureDiscoverer{structureDisc})
	if err != nil {
		t.Fatalf("PlanWithStructure failed: %v", err)
	}

	// Verify compilation roots were found
	if len(result.CompilationRoots) == 0 {
		t.Fatal("Expected compilation roots to be found")
	}

	if result.CompilationRoots[0].GetType() != "mock" {
		t.Errorf("Expected compilation root type 'mock', got '%s'", result.CompilationRoots[0].GetType())
	}

	// Verify root directory
	resolvedRootDir, _ := filepath.EvalSymlinks(result.CompilationRoots[0].GetRootDir())
	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	if resolvedRootDir != resolvedTempDir {
		t.Errorf("Expected compilation root dir %s, got %s", resolvedTempDir, resolvedRootDir)
	}

	// Verify task was discovered
	tasks := result.Graph.GetTasks()
	if len(tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(tasks))
	}

	if len(tasks) > 0 && tasks[0].Name() != "mock-task" {
		t.Errorf("Expected task name 'mock-task', got '%s'", tasks[0].Name())
	}
}

func TestPlanWithStructure_NoCompilationRoot(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "structure_plan_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create a structure discoverer that never finds a compilation root
	structureDisc := &MockStructureDiscoverer{
		name: "MockStructure",
		checkFunc: func(dir string) CompilationRoot {
			return nil // Never find a compilation root
		},
	}

	// Test planning - should succeed but find no tasks since no compilation root is found
	ctx := context.Background()
	result, err := PlanWithStructure(ctx, tempDir, []Discoverer{}, []StructureDiscoverer{structureDisc})
	if err != nil {
		t.Fatalf("PlanWithStructure failed: %v", err)
	}

	// Should have no compilation roots and no tasks
	if len(result.CompilationRoots) != 0 {
		t.Errorf("Expected 0 compilation roots, got %d", len(result.CompilationRoots))
	}

	tasks := result.Graph.GetTasks()
	if len(tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(tasks))
	}
}

func TestPlanWithStructure_TraversesUpwards(t *testing.T) {
	// Create temporary directory structure
	tempDir, err := os.MkdirTemp("", "structure_plan_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create nested directory structure
	deepDir := filepath.Join(tempDir, "level1", "level2", "level3")
	err = os.MkdirAll(deepDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	// Create structure discoverer that recognizes tempDir as compilation root
	structureDisc := &MockStructureDiscoverer{
		name: "MockStructure",
		checkFunc: func(dir string) CompilationRoot {
			resolvedDir, _ := filepath.EvalSymlinks(dir)
			resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
			if resolvedDir == resolvedTempDir {
				return &MockCompilationRoot{
					rootDir:  dir,
					rootType: "mock",
				}
			}
			return nil
		},
	}

	// Create a simple discoverer
	discoverer := NewMockPlanDiscoverer("MockDiscoverer",
		func(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *BuildContext) (*DiscoveryResult, error) {
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})

	// Test planning from the deep directory - should traverse upwards to find compilation root
	ctx := context.Background()
	result, err := PlanWithStructure(ctx, deepDir, []Discoverer{discoverer}, []StructureDiscoverer{structureDisc})
	if err != nil {
		t.Fatalf("PlanWithStructure failed: %v", err)
	}

	// Verify compilation root was found by traversing upwards
	if len(result.CompilationRoots) == 0 {
		t.Fatal("Expected compilation root to be found by traversing upwards")
	}

	resolvedRootDir, _ := filepath.EvalSymlinks(result.CompilationRoots[0].GetRootDir())
	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	if resolvedRootDir != resolvedTempDir {
		t.Errorf("Expected compilation root dir %s, got %s", resolvedTempDir, resolvedRootDir)
	}
}