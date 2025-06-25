package kotlin

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"fbs/pkg/graph"
)

// JunitTest represents a task that runs JUnit tests for a specific Kotlin test file
type JunitTest struct {
	testFile     string
	sourceDir    string
	className    string
	dependencies []graph.Task
}

// NewJunitTest creates a new JUnit test task
func NewJunitTest(testFile, sourceDir, className string) *JunitTest {
	return &JunitTest{
		testFile:     testFile,
		sourceDir:    sourceDir,
		className:    className,
		dependencies: []graph.Task{},
	}
}

// ID returns the unique identifier for this task (using hash)
func (j *JunitTest) ID() string {
	return j.Hash()
}

// Name returns the human-readable name for this task type
func (j *JunitTest) Name() string {
	return "junit-test"
}

// Directory returns the directory where this task was discovered
func (j *JunitTest) Directory() string {
	return j.sourceDir
}

// TaskType returns the type of task (test for JUnit tests)
func (j *JunitTest) TaskType() graph.TaskType {
	return graph.TaskTypeTest
}

// Hash returns a hash representing the task's configuration and inputs
func (j *JunitTest) Hash() string {
	h := sha256.New()
	
	// Include task type and test file info
	h.Write([]byte("JunitTest"))
	h.Write([]byte(j.testFile))
	h.Write([]byte(j.sourceDir))
	h.Write([]byte(j.className))
	
	// Include test file modification time if file exists
	if info, err := os.Stat(filepath.Join(j.sourceDir, j.testFile)); err == nil {
		h.Write([]byte(fmt.Sprintf("%d", info.ModTime().Unix())))
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Dependencies returns the list of tasks that must complete before this task can run
func (j *JunitTest) Dependencies() []graph.Task {
	return j.dependencies
}

// Execute runs the JUnit test task
func (j *JunitTest) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	// Create test results directory
	resultsDir := filepath.Join(workDir, "test-results")
	if err := os.MkdirAll(resultsDir, 0755); err != nil {
		return graph.TaskResult{Error: fmt.Errorf("failed to create test results directory: %w", err)}
	}
	
	// Build classpath from dependency inputs
	var classpathParts []string
	for _, dep := range dependencyInputs {
		// Add compiled classes directories
		classesDir := filepath.Join(dep.OutputDir, "classes")
		if _, err := os.Stat(classesDir); err == nil {
			classpathParts = append(classpathParts, classesDir)
		}
		
		// Add JAR files from dependencies
		for _, file := range dep.Files {
			if strings.HasSuffix(file, ".jar") {
				var jarPath string
				if filepath.IsAbs(file) {
					// Absolute path (e.g., from artifact downloads)
					jarPath = file
				} else {
					// Relative path (from other build tasks)
					jarPath = filepath.Join(dep.OutputDir, file)
				}
				if _, err := os.Stat(jarPath); err == nil {
					classpathParts = append(classpathParts, jarPath)
				}
			}
		}
	}
	
	classpath := strings.Join(classpathParts, ":")
	
	// Build java command to run JUnit tests
	args := []string{
		"-cp", classpath,
		"org.junit.platform.console.ConsoleLauncher",
		"--select-class", j.className,
		"--reports-dir", resultsDir,
	}
	
	// Execute java command
	cmd := exec.CommandContext(ctx, "java", args...)
	cmd.Dir = workDir
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("junit test execution failed: %w\nOutput: %s", err, string(output)),
		}
	}
	
	// List generated test result files
	var resultFiles []string
	err = filepath.Walk(resultsDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(workDir, path)
			if err != nil {
				return err
			}
			resultFiles = append(resultFiles, relPath)
		}
		return nil
	})
	
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to enumerate test result files: %w", err),
		}
	}
	
	return graph.TaskResult{
		Files: resultFiles,
	}
}

// AddDependency adds a task as a dependency
func (j *JunitTest) AddDependency(task graph.Task) {
	j.dependencies = append(j.dependencies, task)
}

// GetTestFile returns the test file path
func (j *JunitTest) GetTestFile() string {
	return j.testFile
}

// GetSourceDir returns the source directory
func (j *JunitTest) GetSourceDir() string {
	return j.sourceDir
}

// GetClassName returns the class name being tested
func (j *JunitTest) GetClassName() string {
	return j.className
}

// DisplayName returns a detailed display name including the test file
func (j *JunitTest) DisplayName() string {
	return fmt.Sprintf("junit-test (%s)", j.testFile)
}