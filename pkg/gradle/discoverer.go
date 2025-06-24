package gradle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

// GradleDiscoverer discovers Gradle project tasks from build.gradle.kt files
type GradleDiscoverer struct{}

// NewGradleDiscoverer creates a new Gradle project discoverer
func NewGradleDiscoverer() *GradleDiscoverer {
	return &GradleDiscoverer{}
}

// Name returns the name of this discoverer
func (d *GradleDiscoverer) Name() string {
	return "GradleDiscoverer"
}

// Discover finds build.gradle.kt files and creates Gradle project tasks
// It also injects KotlinCompile tasks as dependencies of JunitTest tasks in subdirectories
func (d *GradleDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task) (*discoverer.DiscoveryResult, error) {
	// Check if path exists
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// Path doesn't exist, return empty result
			return &discoverer.DiscoveryResult{
				Tasks: []graph.Task{},
				Path:  path,
			}, nil
		}
		return nil, fmt.Errorf("failed to stat path %s: %w", path, err)
	}
	
	var searchDir string
	if info.IsDir() {
		searchDir = path
	} else {
		// If it's a file, use its directory
		searchDir = filepath.Dir(path)
	}
	
	// Check if build.gradle.kt exists in this directory
	buildFile := "build.gradle.kts"
	buildFilePath := filepath.Join(searchDir, buildFile)
	if _, err := os.Stat(buildFilePath); os.IsNotExist(err) {
		// No build.gradle.kts file, return empty result
		return &discoverer.DiscoveryResult{
			Tasks: []graph.Task{},
			Path:  path,
		}, nil
	}
	
	// Create Gradle project task
	task := NewGradleProject(searchDir, buildFile)
	
	// Process potential dependencies to inject KotlinCompile tasks into JunitTest tasks
	d.injectDependencies(potentialDependencies)
	
	return &discoverer.DiscoveryResult{
		Tasks: []graph.Task{task},
		Path:  path,
	}, nil
}

// injectDependencies finds JunitTest tasks and injects all KotlinCompile tasks as their dependencies
func (d *GradleDiscoverer) injectDependencies(potentialDependencies []graph.Task) {
	// Separate KotlinCompile and JunitTest tasks
	var kotlinCompileTasks []*kotlin.KotlinCompile
	var junitTestTasks []*kotlin.JunitTest
	
	for _, task := range potentialDependencies {
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
			if !d.hasDependency(junitTask, kotlinTask) {
				junitTask.AddDependency(kotlinTask)
			}
		}
	}
}

// hasDependency checks if a JunitTest task already has a specific KotlinCompile task as a dependency
func (d *GradleDiscoverer) hasDependency(junitTask *kotlin.JunitTest, kotlinTask *kotlin.KotlinCompile) bool {
	for _, dep := range junitTask.Dependencies() {
		if dep.ID() == kotlinTask.ID() {
			return true
		}
	}
	return false
}