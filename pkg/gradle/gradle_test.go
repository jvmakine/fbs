package gradle

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

func TestGradleDiscoverer_Discover(t *testing.T) {
	// Create temporary directory for testing
	tempDir, err := os.MkdirTemp("", "gradle_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	gradleDiscoverer := NewGradleDiscoverer()
	ctx := context.Background()

	// Test 1: Directory with build.gradle.kts file
	gradleDir := filepath.Join(tempDir, "gradle_project")
	err = os.MkdirAll(gradleDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create gradle project dir: %v", err)
	}

	// Create build.gradle.kts file
	buildFile := "build.gradle.kts"
	buildContent := `plugins {
    kotlin("jvm") version "1.9.20"
}

repositories {
    mavenCentral()
}

dependencies {
    testImplementation("org.junit.jupiter:junit-jupiter:5.10.0")
}`
	err = os.WriteFile(filepath.Join(gradleDir, buildFile), []byte(buildContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create build file: %v", err)
	}

	buildContext := discoverer.NewBuildContext()
	result, err := gradleDiscoverer.Discover(ctx, gradleDir, []graph.Task{}, buildContext)
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
	gradleTask, ok := task.(*GradleProject)
	if !ok {
		t.Errorf("Expected GradleProject task, got %T", task)
	}

	if gradleTask.GetBuildFile() != "build.gradle.kts" {
		t.Errorf("Expected build file 'build.gradle.kts', got '%s'", gradleTask.GetBuildFile())
	}

	// Test 2: Directory without build.gradle.kts file
	emptyDir := filepath.Join(tempDir, "empty_project")
	err = os.MkdirAll(emptyDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create empty dir: %v", err)
	}

	result, err = gradleDiscoverer.Discover(ctx, emptyDir, []graph.Task{}, buildContext)
	if err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	if len(result.Tasks) != 0 {
		t.Errorf("Expected 0 tasks for empty dir, got %d", len(result.Tasks))
	}
}

func TestGradleDiscoverer_Name(t *testing.T) {
	discoverer := NewGradleDiscoverer()
	if discoverer.Name() != "GradleDiscoverer" {
		t.Errorf("Expected name 'GradleDiscoverer', got '%s'", discoverer.Name())
	}
}

func TestGradleProject_BasicProperties(t *testing.T) {
	task := NewGradleProject("/project", "build.gradle.kts")

	// ID is now hash-based, so we just check it's not empty
	if task.ID() == "" {
		t.Error("Expected non-empty ID")
	}

	if task.GetProjectDir() != "/project" {
		t.Errorf("Expected project dir '/project', got '%s'", task.GetProjectDir())
	}

	if task.GetBuildFile() != "build.gradle.kts" {
		t.Errorf("Expected build file 'build.gradle.kts', got '%s'", task.GetBuildFile())
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
	task2 := NewGradleProject("/project2", "build.gradle.kts")
	if task.Hash() == task2.Hash() {
		t.Error("Different tasks should have different hashes")
	}
}

func TestGradleDiscoverer_DependencyInjection(t *testing.T) {
	discoverer := NewGradleDiscoverer()

	// Create mock tasks
	kotlinTask1 := kotlin.NewKotlinCompile("/src", []string{"Main.kt"})
	kotlinTask2 := kotlin.NewKotlinCompile("/lib", []string{"Utils.kt"})
	junitTask1 := kotlin.NewJunitTest("MainTest.kt", "/test", "MainTest")
	junitTask2 := kotlin.NewJunitTest("UtilsTest.kt", "/test", "UtilsTest")

	// Initially JUnit tasks should have no dependencies
	if len(junitTask1.Dependencies()) != 0 {
		t.Errorf("Expected junit task 1 to have 0 initial dependencies, got %d", len(junitTask1.Dependencies()))
	}
	if len(junitTask2.Dependencies()) != 0 {
		t.Errorf("Expected junit task 2 to have 0 initial dependencies, got %d", len(junitTask2.Dependencies()))
	}

	// Create list of potential dependencies
	potentialDeps := []graph.Task{kotlinTask1, kotlinTask2, junitTask1, junitTask2}

	// Call dependency injection
	discoverer.injectDependencies(potentialDeps)

	// Now JUnit tasks should have Kotlin compile tasks as dependencies
	if len(junitTask1.Dependencies()) != 2 {
		t.Errorf("Expected junit task 1 to have 2 dependencies after injection, got %d", len(junitTask1.Dependencies()))
	}
	if len(junitTask2.Dependencies()) != 2 {
		t.Errorf("Expected junit task 2 to have 2 dependencies after injection, got %d", len(junitTask2.Dependencies()))
	}

	// Verify the dependencies are the Kotlin compile tasks
	deps1 := junitTask1.Dependencies()
	foundKotlin1 := false
	foundKotlin2 := false
	for _, dep := range deps1 {
		if dep.Name() == "kotlin-compile" && dep.Directory() == "/src" {
			foundKotlin1 = true
		}
		if dep.Name() == "kotlin-compile" && dep.Directory() == "/lib" {
			foundKotlin2 = true
		}
	}
	if !foundKotlin1 || !foundKotlin2 {
		t.Error("JUnit task should have both Kotlin compile tasks as dependencies")
	}
}