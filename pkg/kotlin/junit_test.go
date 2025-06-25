package kotlin

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
)

func TestJunitDiscoverer_Discover(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "junit_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	jd := NewJunitDiscoverer()
	ctx := context.Background()

	// Test 1: Directory with JUnit test files
	testDir := filepath.Join(tempDir, "test_project", "src", "test", "kotlin")
	err = os.MkdirAll(testDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create test project dir: %v", err)
	}

	// Create a test file with JUnit annotations
	testFile := "ExampleTest.kt"
	testContent := `import org.junit.jupiter.api.Test
import org.junit.jupiter.api.Assertions.assertEquals

class ExampleTest {
    @Test
    fun testSomething() {
        assertEquals(2 + 2, 4)
    }
}`
	err = os.WriteFile(filepath.Join(testDir, testFile), []byte(testContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	// Create a Kotlin file in src/test that doesn't end with Test.kt
	nonTestInTestDir := "Helper.kt"
	helperContent := `class Helper {
    fun helperMethod() {
        // This is a helper class, not a test
    }
}`
	err = os.WriteFile(filepath.Join(testDir, nonTestInTestDir), []byte(helperContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create helper file: %v", err)
	}

	// Create a non-test Kotlin file in src/main
	srcMainDir := filepath.Join(tempDir, "test_project", "src", "main", "kotlin")
	err = os.MkdirAll(srcMainDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create src/main dir: %v", err)
	}
	
	nonTestFile := "Example.kt"
	nonTestContent := `class Example {
    fun hello(): String {
        return "Hello"
    }
}`
	err = os.WriteFile(filepath.Join(srcMainDir, nonTestFile), []byte(nonTestContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create non-test file: %v", err)
	}

	buildContext := discoverer.NewBuildContext()
	result, err := jd.Discover(ctx, testDir, []graph.Task{}, buildContext)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 1 {
		t.Errorf("Expected 1 test task, got %d", len(result.Tasks))
	}

	if len(result.Errors) != 0 {
		t.Errorf("Expected no errors, got %d: %v", len(result.Errors), result.Errors)
	}

	task := result.Tasks[0]
	junitTask, ok := task.(*JunitTest)
	if !ok {
		t.Errorf("Expected JunitTest task, got %T", task)
	}

	if junitTask.GetClassName() != "ExampleTest" {
		t.Errorf("Expected class name 'ExampleTest', got '%s'", junitTask.GetClassName())
	}

	// Test 2: Directory with no test files
	emptyDir := filepath.Join(tempDir, "empty_project")
	err = os.MkdirAll(emptyDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create empty dir: %v", err)
	}

	result, err = jd.Discover(ctx, emptyDir, []graph.Task{}, buildContext)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks for empty dir, got %d", len(result.Tasks))
	}
}

func TestJunitDiscoverer_Name(t *testing.T) {
	discoverer := NewJunitDiscoverer()
	if discoverer.Name() != "JunitDiscoverer" {
		t.Errorf("Expected name 'JunitDiscoverer', got '%s'", discoverer.Name())
	}
}

func TestJunitTest_BasicProperties(t *testing.T) {
	task := NewJunitTest("ExampleTest.kt", "/test/src", "ExampleTest")

	// ID is now hash-based, so we just check it's not empty
	if task.ID() == "" {
		t.Error("Expected non-empty ID")
	}

	if task.GetTestFile() != "ExampleTest.kt" {
		t.Errorf("Expected test file 'ExampleTest.kt', got '%s'", task.GetTestFile())
	}

	if task.GetSourceDir() != "/test/src" {
		t.Errorf("Expected source dir '/test/src', got '%s'", task.GetSourceDir())
	}

	if task.GetClassName() != "ExampleTest" {
		t.Errorf("Expected class name 'ExampleTest', got '%s'", task.GetClassName())
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
	task2 := NewJunitTest("ExampleTest.kt", "/test/src2", "ExampleTest")
	if task.Hash() == task2.Hash() {
		t.Error("Different tasks should have different hashes")
	}
}

func TestJunitTest_AddDependency(t *testing.T) {
	junitTask := NewJunitTest("ExampleTest.kt", "/test/src", "ExampleTest")
	kotlinTask := NewKotlinCompile("/src", []string{"Example.kt"})

	// Initially no dependencies
	if len(junitTask.Dependencies()) != 0 {
		t.Errorf("Expected 0 initial dependencies, got %d", len(junitTask.Dependencies()))
	}

	// Add dependency
	junitTask.AddDependency(kotlinTask)

	if len(junitTask.Dependencies()) != 1 {
		t.Errorf("Expected 1 dependency after adding, got %d", len(junitTask.Dependencies()))
	}

	if junitTask.Dependencies()[0].Name() != "kotlin-compile" {
		t.Errorf("Expected dependency name 'kotlin-compile', got '%s'", junitTask.Dependencies()[0].Name())
	}
}
