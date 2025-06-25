package gradle

import (
	"context"
	"os"
	"path/filepath"

	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

// GradleStructureDiscoverer discovers Gradle compilation roots
type GradleStructureDiscoverer struct{}

// NewGradleStructureDiscoverer creates a new Gradle structure discoverer
func NewGradleStructureDiscoverer() *GradleStructureDiscoverer {
	return &GradleStructureDiscoverer{}
}

// Name returns the name of this structure discoverer
func (d *GradleStructureDiscoverer) Name() string {
	return "GradleStructureDiscoverer"
}

// IsCompilationRoot checks if the directory contains a build.gradle.kt file
func (d *GradleStructureDiscoverer) IsCompilationRoot(ctx context.Context, dir string) (discoverer.CompilationRoot, error) {
	buildFile := filepath.Join(dir, "build.gradle.kts")
	if _, err := os.Stat(buildFile); err != nil {
		// No build.gradle.kts file found
		return nil, nil
	}
	
	// This is a Gradle compilation root, create and return it
	root := NewGradleCompilationRoot(dir)
	return root, nil
}

// GradleCompilationRoot represents a Gradle project compilation root
type GradleCompilationRoot struct {
	rootDir string
	versions *GradleArtefactVersions
}

// NewGradleCompilationRoot creates a new Gradle compilation root
func NewGradleCompilationRoot(rootDir string) *GradleCompilationRoot {
	root := &GradleCompilationRoot{
		rootDir: rootDir,
	}
	
	// Try to load version catalog from the project root
	root.loadVersionCatalog()
	
	return root
}

// GetRootDir returns the root directory of this compilation root
func (g *GradleCompilationRoot) GetRootDir() string {
	return g.rootDir
}

// GetType returns the type of compilation root
func (g *GradleCompilationRoot) GetType() string {
	return "gradle"
}

// GetBuildContext returns a BuildContext with Gradle-specific metadata
func (g *GradleCompilationRoot) GetBuildContext(dir string) *discoverer.BuildContext {
	context := discoverer.NewBuildContext()
	
	if g.versions != nil {
		context.Set(g.versions)
	}
	
	return context
}

// GetTaskDependencies returns task dependencies for the given directory and discovered tasks
func (g *GradleCompilationRoot) GetTaskDependencies(dir string, tasks []graph.Task) []graph.Task {
	// Separate KotlinCompile and JunitTest tasks
	var kotlinCompileTasks []*kotlin.KotlinCompile
	var junitTestTasks []*kotlin.JunitTest
	
	for _, task := range tasks {
		switch t := task.(type) {
		case *kotlin.KotlinCompile:
			kotlinCompileTasks = append(kotlinCompileTasks, t)
		case *kotlin.JunitTest:
			junitTestTasks = append(junitTestTasks, t)
		}
	}
	
	// Inject all KotlinCompile tasks as dependencies of all JunitTest tasks
	for _, junitTask := range junitTestTasks {
		for _, kotlinTask := range kotlinCompileTasks {
			// Check if this dependency doesn't already exist
			if !g.hasDependency(junitTask, kotlinTask) {
				junitTask.AddDependency(kotlinTask)
			}
		}
	}
	
	// Return all tasks (they have been modified in place with dependencies)
	return tasks
}

// loadVersionCatalog loads the Gradle version catalog if it exists
func (g *GradleCompilationRoot) loadVersionCatalog() {
	versionCatalogPath := filepath.Join(g.rootDir, "gradle", "libs.versions.toml")
	if _, err := os.Stat(versionCatalogPath); err != nil {
		return // No version catalog found
	}
	
	contextDiscoverer := NewGradleContextDiscoverer()
	versions, err := contextDiscoverer.parseVersionCatalog(versionCatalogPath)
	if err != nil {
		return // Failed to parse, continue without versions
	}
	
	versions.ProjectDir = g.rootDir
	g.versions = versions
}

// hasDependency checks if a JunitTest task already has a specific KotlinCompile task as a dependency
func (g *GradleCompilationRoot) hasDependency(junitTask *kotlin.JunitTest, kotlinTask *kotlin.KotlinCompile) bool {
	for _, dep := range junitTask.Dependencies() {
		if dep.ID() == kotlinTask.ID() {
			return true
		}
	}
	return false
}