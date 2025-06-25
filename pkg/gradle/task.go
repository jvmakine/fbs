package gradle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"fbs/pkg/graph"
)

// GradleProject represents a task that manages a Gradle project build
type GradleProject struct {
	projectDir   string
	buildFile    string
	dependencies []graph.Task
}

// NewGradleProject creates a new Gradle project task
func NewGradleProject(projectDir, buildFile string) *GradleProject {
	return &GradleProject{
		projectDir:   projectDir,
		buildFile:    buildFile,
		dependencies: []graph.Task{},
	}
}

// ID returns the unique identifier for this task (using hash)
func (g *GradleProject) ID() string {
	return g.Hash()
}

// Name returns the human-readable name for this task type
func (g *GradleProject) Name() string {
	return "gradle-project"
}

// Directory returns the directory where this task was discovered
func (g *GradleProject) Directory() string {
	return g.projectDir
}

// TaskType returns the type of task (build for gradle projects)
func (g *GradleProject) TaskType() graph.TaskType {
	return graph.TaskTypeBuild
}

// Hash returns a hash representing the task's configuration and inputs
func (g *GradleProject) Hash() string {
	h := sha256.New()
	
	// Include task type and project directory
	h.Write([]byte("GradleProject"))
	h.Write([]byte(g.projectDir))
	h.Write([]byte(g.buildFile))
	
	// Include build file modification time if file exists
	if info, err := os.Stat(filepath.Join(g.projectDir, g.buildFile)); err == nil {
		h.Write([]byte(fmt.Sprintf("%d", info.ModTime().Unix())))
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}

// Dependencies returns the list of tasks that must complete before this task can run
func (g *GradleProject) Dependencies() []graph.Task {
	return g.dependencies
}

// Execute runs the Gradle project build
func (g *GradleProject) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	// Create build output directory
	buildDir := filepath.Join(workDir, "gradle-build")
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return graph.TaskResult{Error: fmt.Errorf("failed to create build directory: %w", err)}
	}
	
	// Execute gradle build command
	// This assumes gradle wrapper is available in the project
	gradleCmd := "./gradlew"
	if _, err := os.Stat(filepath.Join(g.projectDir, "gradlew")); os.IsNotExist(err) {
		// Fall back to system gradle
		gradleCmd = "gradle"
	}
	
	args := []string{"build", "--build-cache"}
	
	// Execute gradle command
	cmd := exec.CommandContext(ctx, gradleCmd, args...)
	cmd.Dir = g.projectDir
	
	output, err := cmd.CombinedOutput()
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("gradle build failed: %w\nOutput: %s", err, string(output)),
		}
	}
	
	// Copy build outputs to work directory
	buildOutputDir := filepath.Join(g.projectDir, "build")
	if _, err := os.Stat(buildOutputDir); err == nil {
		// Copy relevant build artifacts
		err = copyDirectory(buildOutputDir, buildDir)
		if err != nil {
			return graph.TaskResult{
				Error: fmt.Errorf("failed to copy build outputs: %w", err),
			}
		}
	}
	
	// List generated build files
	var buildFiles []string
	err = filepath.Walk(buildDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			relPath, err := filepath.Rel(workDir, path)
			if err != nil {
				return err
			}
			buildFiles = append(buildFiles, relPath)
		}
		return nil
	})
	
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to enumerate build files: %w", err),
		}
	}
	
	return graph.TaskResult{
		Files: buildFiles,
	}
}

// AddDependency adds a task as a dependency
func (g *GradleProject) AddDependency(task graph.Task) {
	g.dependencies = append(g.dependencies, task)
}

// GetProjectDir returns the project directory
func (g *GradleProject) GetProjectDir() string {
	return g.projectDir
}

// GetBuildFile returns the build file name
func (g *GradleProject) GetBuildFile() string {
	return g.buildFile
}

// DisplayName returns a detailed display name
func (g *GradleProject) DisplayName() string {
	return g.Name()
}

// copyDirectory recursively copies a directory
func copyDirectory(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		
		dstPath := filepath.Join(dst, relPath)
		
		if info.IsDir() {
			return os.MkdirAll(dstPath, info.Mode())
		}
		
		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()
	
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	
	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()
	
	_, err = srcFile.WriteTo(dstFile)
	return err
}