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
		// Parse the JUnit output to extract clean failure information
		cleanError := j.parseJUnitFailure(string(output))
		return graph.TaskResult{
			Error: fmt.Errorf("junit test execution failed: %w\n%s", err, cleanError),
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

// parseJUnitFailure extracts clean failure information from JUnit output
func (j *JunitTest) parseJUnitFailure(output string) string {
	lines := strings.Split(output, "\n")
	var failureLines []string
	inFailureSection := false
	inStackTrace := false
	
	for _, line := range lines {
		// Skip JUnit header and promotional content
		if strings.Contains(line, "Thanks for using JUnit") ||
		   strings.Contains(line, "sponsoring") ||
		   strings.Contains(line, "Test run finished") ||
		   strings.Contains(line, "containers found") ||
		   strings.Contains(line, "containers skipped") ||
		   strings.Contains(line, "containers started") ||
		   strings.Contains(line, "containers aborted") ||
		   strings.Contains(line, "containers successful") ||
		   strings.Contains(line, "containers failed") ||
		   strings.Contains(line, "tests found") ||
		   strings.Contains(line, "tests skipped") ||
		   strings.Contains(line, "tests started") ||
		   strings.Contains(line, "tests aborted") ||
		   strings.Contains(line, "tests successful") ||
		   strings.Contains(line, "tests failed") ||
		   strings.Contains(line, "WARNING: Delegated") ||
		   strings.Contains(line, "This behaviour has been deprecated") ||
		   strings.Contains(line, "Please use the 'execute' command") ||
		   strings.HasPrefix(line, "╷") ||
		   strings.HasPrefix(line, "├─") ||
		   strings.HasPrefix(line, "│") ||
		   strings.HasPrefix(line, "└─") ||
		   strings.TrimSpace(line) == "" {
			continue
		}
		
		// Start capturing when we hit "Failures"
		if strings.HasPrefix(line, "Failures (") {
			inFailureSection = true
			continue
		}
		
		// Start capturing stack trace when we see the test method line  
		if inFailureSection && (strings.Contains(line, "JUnit Jupiter:") || strings.Contains(line, "MethodSource") || strings.Contains(line, "=>")) {
			inStackTrace = true
			failureLines = append(failureLines, line)
			continue
		}
		
		// Capture stack trace lines
		if inStackTrace {
			// Trim excessive whitespace but preserve indentation structure
			trimmed := strings.TrimSpace(line)
			if trimmed != "" {
				// Preserve some indentation for readability
				if strings.HasPrefix(trimmed, "=>") || strings.HasPrefix(trimmed, "MethodSource") {
					failureLines = append(failureLines, "    "+trimmed)
				} else if strings.Contains(trimmed, ".java:") || strings.Contains(trimmed, ".kt:") {
					failureLines = append(failureLines, "       "+trimmed)
				} else {
					failureLines = append(failureLines, "    "+trimmed)
				}
			}
		}
	}
	
	if len(failureLines) == 0 {
		// If we couldn't parse the failure, return a simplified version
		return "Test failed (see full output above for details)"
	}
	
	return strings.Join(failureLines, "\n")
}