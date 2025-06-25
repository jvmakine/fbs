package gradle

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

// JarCompile represents a task that compiles Kotlin sources into a JAR file
type JarCompile struct {
	projectDir   string
	outputPath   string
	mainSources  []string
	dependencies []graph.Task
	id           string
	hash         string
}

// NewJarCompile creates a new JAR compilation task
func NewJarCompile(projectDir string, mainSources []string) *JarCompile {
	task := &JarCompile{
		projectDir:   projectDir,
		mainSources:  mainSources,
		dependencies: []graph.Task{},
	}
	
	// Generate output path
	projectName := filepath.Base(projectDir)
	task.outputPath = filepath.Join(projectDir, "build", "libs", projectName+".jar")
	
	// Generate ID and hash
	task.id = task.generateID()
	task.hash = task.generateHash()
	
	return task
}

// ID returns the unique identifier for this task
func (j *JarCompile) ID() string {
	return j.id
}

// Name returns the human-readable name of this task
func (j *JarCompile) Name() string {
	return "jar-compile"
}

// Hash returns a hash representing the task's configuration
func (j *JarCompile) Hash() string {
	return j.hash
}

// Dependencies returns the list of tasks this task depends on
func (j *JarCompile) Dependencies() []graph.Task {
	return j.dependencies
}

// AddDependency adds a dependency to this task
func (j *JarCompile) AddDependency(task graph.Task) {
	j.dependencies = append(j.dependencies, task)
}

// Directory returns the directory this task operates in
func (j *JarCompile) Directory() string {
	return j.projectDir
}

// TaskType returns the type of this task
func (j *JarCompile) TaskType() graph.TaskType {
	return graph.TaskTypeBuild
}

// Execute runs the JAR compilation task
func (j *JarCompile) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	// Create output directory
	outputDir := filepath.Dir(j.outputPath)
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to create output directory: %w", err),
		}
	}
	
	// Collect all .class files from dependency inputs
	var classFiles []string
	for _, depInput := range dependencyInputs {
		for _, file := range depInput.Files {
			if strings.HasSuffix(file, ".class") {
				fullPath := filepath.Join(depInput.OutputDir, file)
				classFiles = append(classFiles, fullPath)
			}
		}
	}
	
	if len(classFiles) == 0 {
		return graph.TaskResult{
			Error: fmt.Errorf("no compiled classes found to package"),
		}
	}
	
	// Create JAR file using jar command
	cmd := exec.CommandContext(ctx, "jar", "cf", j.outputPath)
	
	// Find the common classes directory to work from
	var classesDir string
	if len(classFiles) > 0 {
		// Look for the classes directory in the path
		firstClassFile := classFiles[0]
		if strings.Contains(firstClassFile, "/classes/") {
			classesDir = firstClassFile[:strings.Index(firstClassFile, "/classes/")+9] // Include "/classes/"
		} else {
			classesDir = filepath.Dir(firstClassFile)
		}
	}
	
	// Set working directory to the classes directory
	if classesDir != "" {
		cmd.Dir = classesDir
		
		// Add all class files relative to the classes directory
		for _, classFile := range classFiles {
			relPath, err := filepath.Rel(classesDir, classFile)
			if err != nil {
				relPath = filepath.Base(classFile) // Just use filename as fallback
			}
			cmd.Args = append(cmd.Args, relPath)
		}
	} else {
		// Fallback: add files directly
		cmd.Args = append(cmd.Args, classFiles...)
	}
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("jar compilation failed: %w\nOutput: %s", err, string(output)),
		}
	}
	
	// Return the JAR file as output (absolute path for external dependencies)
	return graph.TaskResult{
		Files: []string{j.outputPath},
	}
}

// GetOutputPath returns the path where the JAR file will be created
func (j *JarCompile) GetOutputPath() string {
	return j.outputPath
}

// GetProjectDir returns the project directory
func (j *JarCompile) GetProjectDir() string {
	return j.projectDir
}

// generateID creates a unique ID for this task
func (j *JarCompile) generateID() string {
	hasher := sha256.New()
	hasher.Write([]byte("jar-compile"))
	hasher.Write([]byte(j.projectDir))
	for _, source := range j.mainSources {
		hasher.Write([]byte(source))
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// generateHash creates a hash for this task's configuration
func (j *JarCompile) generateHash() string {
	hasher := sha256.New()
	hasher.Write([]byte(j.projectDir))
	hasher.Write([]byte(j.outputPath))
	for _, source := range j.mainSources {
		hasher.Write([]byte(source))
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}