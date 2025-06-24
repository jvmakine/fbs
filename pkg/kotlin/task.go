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
	id          string
	sourceDir   string
	kotlinFiles []string
	classpath   []string
}

// NewKotlinCompile creates a new Kotlin compilation task
func NewKotlinCompile(id, sourceDir string, kotlinFiles []string) *KotlinCompile {
	return &KotlinCompile{
		id:          id,
		sourceDir:   sourceDir,
		kotlinFiles: kotlinFiles,
		classpath:   []string{},
	}
}

// ID returns the unique identifier for this task
func (k *KotlinCompile) ID() string {
	return k.id
}

// Hash returns a hash representing the task's configuration and inputs
func (k *KotlinCompile) Hash() string {
	h := sha256.New()
	
	// Include task type, ID, and source directory
	h.Write([]byte("KotlinCompile"))
	h.Write([]byte(k.id))
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
	// No dependencies for basic Kotlin compilation
	return []graph.Task{}
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
	
	// Add classpath if provided
	if len(k.classpath) > 0 {
		args = append(args, "-classpath", strings.Join(k.classpath, ":"))
	}
	
	// Add dependency classpaths
	for _, dep := range dependencyInputs {
		depClassesDir := filepath.Join(dep.OutputDir, "classes")
		if _, err := os.Stat(depClassesDir); err == nil {
			k.classpath = append(k.classpath, depClassesDir)
		}
	}
	
	if len(k.classpath) > 0 {
		args = append(args, "-classpath", strings.Join(k.classpath, ":"))
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