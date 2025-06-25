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

// KotlinCompile represents a task that compiles Kotlin source files
type KotlinCompile struct {
	sourceDir    string
	kotlinFiles  []string
	classpath    []string
	dependencies []graph.Task
}

// NewKotlinCompile creates a new Kotlin compilation task
func NewKotlinCompile(sourceDir string, kotlinFiles []string) *KotlinCompile {
	return &KotlinCompile{
		sourceDir:    sourceDir,
		kotlinFiles:  kotlinFiles,
		classpath:    []string{},
		dependencies: []graph.Task{},
	}
}

// ID returns the unique identifier for this task (using hash)
func (k *KotlinCompile) ID() string {
	return k.Hash()
}

// Name returns the human-readable name for this task type
func (k *KotlinCompile) Name() string {
	return "kotlin-compile"
}

// Directory returns the directory where this task was discovered
func (k *KotlinCompile) Directory() string {
	return k.sourceDir
}

// TaskType returns the type of task (build for compilation)
func (k *KotlinCompile) TaskType() graph.TaskType {
	return graph.TaskTypeBuild
}

// Hash returns a hash representing the task's configuration and inputs
func (k *KotlinCompile) Hash() string {
	h := sha256.New()
	
	// Include task type and source directory
	h.Write([]byte("KotlinCompile"))
	h.Write([]byte(k.sourceDir))
	
	// Include sorted list of Kotlin files for consistency
	sortedFiles := make([]string, len(k.kotlinFiles))
	copy(sortedFiles, k.kotlinFiles)
	for _, file := range sortedFiles {
		h.Write([]byte(file))
		
		// Include file modification time if file exists
		if info, err := os.Stat(filepath.Join(k.sourceDir, file)); err == nil {
			h.Write([]byte(fmt.Sprintf("%d", info.ModTime().Unix())))
		}
	}
	
	// Include classpath
	for _, cp := range k.classpath {
		h.Write([]byte(cp))
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Dependencies returns the list of tasks that must complete before this task can run
func (k *KotlinCompile) Dependencies() []graph.Task {
	return k.dependencies
}

// Execute runs the Kotlin compilation task
func (k *KotlinCompile) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	// Create classes output directory
	classesDir := filepath.Join(workDir, "classes")
	if err := os.MkdirAll(classesDir, 0755); err != nil {
		return graph.TaskResult{Error: fmt.Errorf("failed to create classes directory: %w", err)}
	}
	
	// Build kotlin compiler command
	args := []string{"-d", classesDir}
	
	// Build classpath from existing classpath and dependencies
	var classpath []string
	classpath = append(classpath, k.classpath...)
	
	// Add dependency classpaths and JAR files
	for _, dep := range dependencyInputs {
		// Check for compiled classes from other compilation tasks
		depClassesDir := filepath.Join(dep.OutputDir, "classes")
		if _, err := os.Stat(depClassesDir); err == nil {
			classpath = append(classpath, depClassesDir)
		}
		
		// Check for JAR files from artifact-download tasks
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
					classpath = append(classpath, jarPath)
				}
			}
		}
	}
	
	// Add classpath to compiler arguments if not empty
	if len(classpath) > 0 {
		args = append(args, "-classpath", strings.Join(classpath, ":"))
	}
	
	// Add source files
	for _, file := range k.kotlinFiles {
		sourcePath := filepath.Join(k.sourceDir, file)
		args = append(args, sourcePath)
	}
	
	// Execute kotlinc command
	cmd := exec.CommandContext(ctx, "kotlinc", args...)
	cmd.Dir = workDir
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("kotlin compilation failed: %w\nOutput: %s", err, string(output)),
		}
	}
	
	// List generated class files
	var classFiles []string
	err = filepath.Walk(classesDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".class") {
			relPath, err := filepath.Rel(workDir, path)
			if err != nil {
				return err
			}
			classFiles = append(classFiles, relPath)
		}
		return nil
	})
	
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to enumerate class files: %w", err),
		}
	}
	
	return graph.TaskResult{
		Files: classFiles,
	}
}

// SetClasspath sets the classpath for compilation
func (k *KotlinCompile) SetClasspath(classpath []string) {
	k.classpath = classpath
}

// GetSourceDir returns the source directory
func (k *KotlinCompile) GetSourceDir() string {
	return k.sourceDir
}

// GetKotlinFiles returns the list of Kotlin files
func (k *KotlinCompile) GetKotlinFiles() []string {
	return k.kotlinFiles
}

// AddDependency adds a task as a dependency
func (k *KotlinCompile) AddDependency(task graph.Task) {
	k.dependencies = append(k.dependencies, task)
}