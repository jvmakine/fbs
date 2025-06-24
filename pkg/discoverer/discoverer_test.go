package discoverer

import (
	"context"
	"testing"

	"fbs/pkg/graph"
)

// MockDiscoverer implements the Discoverer interface for testing
type MockDiscoverer struct {
	name         string
	discoverFunc func(ctx context.Context, path string) (*DiscoveryResult, error)
}

func NewMockDiscoverer(name string, discoverFunc func(context.Context, string) (*DiscoveryResult, error)) *MockDiscoverer {
	return &MockDiscoverer{
		name:         name,
		discoverFunc: discoverFunc,
	}
}

func (m *MockDiscoverer) Name() string {
	return m.name
}

func (m *MockDiscoverer) Discover(ctx context.Context, path string) (*DiscoveryResult, error) {
	if m.discoverFunc != nil {
		return m.discoverFunc(ctx, path)
	}
	return &DiscoveryResult{
		Tasks: []graph.Task{},
		Path:  path,
	}, nil
}

// MockTask for testing (simplified version from graph package)
type MockTask struct {
	id   string
	hash string
}

func (m *MockTask) ID() string {
	return m.id
}

func (m *MockTask) Hash() string {
	return m.hash
}

func (m *MockTask) Dependencies() []graph.Task {
	return nil
}

func (m *MockTask) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	return graph.TaskResult{Files: []string{m.id + ".txt"}}
}

func TestMultiDiscoverer_Discover(t *testing.T) {
	ctx := context.Background()
	
	// Create mock discoverers
	jsDiscoverer := NewMockDiscoverer("JavaScript", 
		func(ctx context.Context, path string) (*DiscoveryResult, error) {
			if path == "package.json" || path == "src/index.js" {
				return &DiscoveryResult{
					Tasks: []graph.Task{
						&MockTask{id: "npm-install", hash: "npm-hash"},
						&MockTask{id: "webpack-build", hash: "webpack-hash"},
					},
					Path: path,
				}, nil
			}
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})
	
	goDiscoverer := NewMockDiscoverer("Go",
		func(ctx context.Context, path string) (*DiscoveryResult, error) {
			if path == "go.mod" || path == "main.go" {
				return &DiscoveryResult{
					Tasks: []graph.Task{
						&MockTask{id: "go-build", hash: "go-hash"},
					},
					Path: path,
				}, nil
			}
			return &DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		})
	
	multiDiscoverer := NewMultiDiscoverer(jsDiscoverer, goDiscoverer)
	
	// Test JavaScript discovery (should find JS tasks)
	result, err := multiDiscoverer.Discover(ctx, "package.json")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if len(result.Tasks) != 2 {
		t.Errorf("Expected 2 tasks for package.json, got %d", len(result.Tasks))
	}
	
	if result.Path != "package.json" {
		t.Errorf("Expected path 'package.json', got '%s'", result.Path)
	}
	
	// Test Go discovery (should find Go tasks)
	result, err = multiDiscoverer.Discover(ctx, "go.mod")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if len(result.Tasks) != 1 {
		t.Errorf("Expected 1 task for go.mod, got %d", len(result.Tasks))
	}
	
	if result.Tasks[0].ID() != "go-build" {
		t.Errorf("Expected task ID 'go-build', got '%s'", result.Tasks[0].ID())
	}
	
	// Test unknown file type (both discoverers return empty)
	result, err = multiDiscoverer.Discover(ctx, "unknown.txt")
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}
	
	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks for unknown.txt, got %d", len(result.Tasks))
	}
}


func TestMultiDiscoverer_AddDiscoverer(t *testing.T) {
	multiDiscoverer := NewMultiDiscoverer()
	
	if len(multiDiscoverer.GetDiscoverers()) != 0 {
		t.Errorf("Expected 0 discoverers initially, got %d", len(multiDiscoverer.GetDiscoverers()))
	}
	
	jsDiscoverer := NewMockDiscoverer("JavaScript", nil)
	
	multiDiscoverer.AddDiscoverer(jsDiscoverer)
	
	if len(multiDiscoverer.GetDiscoverers()) != 1 {
		t.Errorf("Expected 1 discoverer after adding, got %d", len(multiDiscoverer.GetDiscoverers()))
	}
	
	if multiDiscoverer.GetDiscoverers()[0].Name() != "JavaScript" {
		t.Errorf("Expected discoverer name 'JavaScript', got '%s'", multiDiscoverer.GetDiscoverers()[0].Name())
	}
}

func TestMultiDiscoverer_Name(t *testing.T) {
	multiDiscoverer := NewMultiDiscoverer()
	
	if multiDiscoverer.Name() != "MultiDiscoverer" {
		t.Errorf("Expected name 'MultiDiscoverer', got '%s'", multiDiscoverer.Name())
	}
}

func TestDiscoveryResult(t *testing.T) {
	// Test empty discovery result
	result := &DiscoveryResult{
		Tasks: []graph.Task{},
		Path:  "/test/path",
	}
	
	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks, got %d", len(result.Tasks))
	}
	
	if result.Path != "/test/path" {
		t.Errorf("Expected path '/test/path', got '%s'", result.Path)
	}
	
	if len(result.Errors) != 0 {
		t.Errorf("Expected 0 errors, got %d", len(result.Errors))
	}
	
	// Test result with tasks and errors
	mockTask := &MockTask{id: "test-task", hash: "test-hash"}
	result = &DiscoveryResult{
		Tasks:  []graph.Task{mockTask},
		Errors: []error{},
		Path:   "/another/path",
	}
	
	if len(result.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(result.Tasks))
	}
	
	if result.Tasks[0].ID() != "test-task" {
		t.Errorf("Expected task ID 'test-task', got '%s'", result.Tasks[0].ID())
	}
}