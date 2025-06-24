package kotlin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fbs/pkg/graph"
)

func TestKotlinDiscoverer_Discover(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "kotlin_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discoverer := NewKotlinDiscoverer()
	ctx := context.Background()

	// Test 1: Directory with Kotlin files
	kotlinDir := filepath.Join(tempDir, "kotlin_project")
	err = os.MkdirAll(kotlinDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create kotlin project dir: %v", err)
	}

	// Create some Kotlin files
	kotlinFiles := []string{"Main.kt", "Utils.kt", "App.kt"}
	for _, file := range kotlinFiles {
		content := "fun main() { println(\"Hello from " + file + "\") }"
		err = os.WriteFile(filepath.Join(kotlinDir, file), []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", file, err)
		}
	}

	result, err := discoverer.Discover(ctx, kotlinDir, []graph.Task{})
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 1 {
		t.Errorf("Expected 1 task, got %d", len(result.Tasks))
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(result.Errors), result.Errors)
	}

	task := result.Tasks[0]
	kotlinTask, ok := task.(*KotlinCompile)
	if !ok {
		t.Errorf("Expected KotlinCompile task, got %T", task)
	}

	if len(kotlinTask.GetKotlinFiles()) != 3 {
		t.Errorf("Expected 3 Kotlin files, got %d", len(kotlinTask.GetKotlinFiles()))
	}

	// Test 2: Directory with no Kotlin files
	emptyDir := filepath.Join(tempDir, "empty_project")
	err = os.MkdirAll(emptyDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create empty dir: %v", err)
	}

	// Create non-Kotlin files
	err = os.WriteFile(filepath.Join(emptyDir, "README.md"), []byte("# Project"), 0644)
	if err != nil {
		t.Fatalf("Failed to create README.md: %v", err)
	}

	result, err = discoverer.Discover(ctx, emptyDir, []graph.Task{})
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks for empty dir, got %d", len(result.Tasks))
	}

	// Test 3: Non-existent directory
	result, err = discoverer.Discover(ctx, "/non/existent/path", []graph.Task{})
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks for non-existent path, got %d", len(result.Tasks))
	}

	// Test 4: Single Kotlin file path
	singleFile := filepath.Join(kotlinDir, "Main.kt")
	result, err = discoverer.Discover(ctx, singleFile, []graph.Task{})
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 1 {
		t.Errorf("Expected 1 task for single file, got %d", len(result.Tasks))
	}

	// Should still find all Kotlin files in the directory
	task = result.Tasks[0]
	kotlinTask, ok = task.(*KotlinCompile)
	if !ok {
		t.Errorf("Expected KotlinCompile task, got %T", task)
	}

	if len(kotlinTask.GetKotlinFiles()) != 3 {
		t.Errorf("Expected 3 Kotlin files when discovering from single file, got %d", len(kotlinTask.GetKotlinFiles()))
	}
}

func TestKotlinDiscoverer_Name(t *testing.T) {
	discoverer := NewKotlinDiscoverer()
	if discoverer.Name() != "KotlinDiscoverer" {
		t.Errorf("Expected name 'KotlinDiscoverer', got '%s'", discoverer.Name())
	}
}

func TestKotlinCompile_BasicProperties(t *testing.T) {
	kotlinFiles := []string{"Main.kt", "Utils.kt"}
	task := NewKotlinCompile("test-compile", "/test/src", kotlinFiles)

	if task.ID() != "test-compile" {
		t.Errorf("Expected ID 'test-compile', got '%s'", task.ID())
	}

	if task.GetSourceDir() != "/test/src" {
		t.Errorf("Expected source dir '/test/src', got '%s'", task.GetSourceDir())
	}

	if len(task.GetKotlinFiles()) != 2 {
		t.Errorf("Expected 2 Kotlin files, got %d", len(task.GetKotlinFiles()))
	}

	if len(task.Dependencies()) != 0 {
		t.Errorf("Expected 0 dependencies, got %d", len(task.Dependencies()))
	}

	// Test hash is consistent
	hash1 := task.Hash()
	hash2 := task.Hash()
	if hash1 != hash2 {
		t.Error("Hash should be consistent")
	}

	// Test hash is different for different tasks
	task2 := NewKotlinCompile("test-compile-2", "/test/src", kotlinFiles)
	if task.Hash() == task2.Hash() {
		t.Error("Different tasks should have different hashes")
	}
}

func TestKotlinCompile_Execute_MockTest(t *testing.T) {
	// This test verifies the Execute method structure without requiring kotlinc
	tempDir, err := os.MkdirTemp("", "kotlin_execute_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create source directory with Kotlin files
	sourceDir := filepath.Join(tempDir, "src")
	err = os.MkdirAll(sourceDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create source dir: %v", err)
	}

	kotlinFiles := []string{"Main.kt"}
	for _, file := range kotlinFiles {
		content := "fun main() { println(\"Hello World\") }"
		err = os.WriteFile(filepath.Join(sourceDir, file), []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create %s: %v", file, err)
		}
	}

	task := NewKotlinCompile("test-compile", sourceDir, kotlinFiles)

	// Create work directory
	workDir := filepath.Join(tempDir, "work")
	err = os.MkdirAll(workDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create work dir: %v", err)
	}

	ctx := context.Background()
	result := task.Execute(ctx, workDir, []graph.DependencyInput{})

	// The execution will likely fail since kotlinc might not be available,
	// but we can verify the error handling and directory creation
	classesDir := filepath.Join(workDir, "classes")
	if _, err := os.Stat(classesDir); os.IsNotExist(err) {
		t.Error("Classes directory should have been created")
	}

	// Result should either have files (if kotlinc is available) or an error
	if result.Error == nil && len(result.Files) == 0 {
		t.Error("Expected either files or an error from execution")
	}
}

func TestKotlinCompile_Classpath(t *testing.T) {
	task := NewKotlinCompile("test", "/src", []string{"Main.kt"})
	
	// Test initial classpath is empty
	if len(task.classpath) != 0 {
		t.Errorf("Expected empty initial classpath, got %d items", len(task.classpath))
	}
	
	// Test setting classpath
	classpath := []string{"/lib/kotlin-stdlib.jar", "/lib/other.jar"}
	task.SetClasspath(classpath)
	
	if len(task.classpath) != 2 {
		t.Errorf("Expected 2 classpath items, got %d", len(task.classpath))
	}
}