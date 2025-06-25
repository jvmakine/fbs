package discoverer

import (
	"context"

	"fbs/pkg/graph"
)

// CompilationRoot represents a project root that can provide build context and dependency information
type CompilationRoot interface {
	// GetBuildContext returns a BuildContext for the given directory within this compilation root
	GetBuildContext(dir string) *BuildContext
	
	// GetTaskDependencies returns task dependencies that should be added to tasks discovered in the given directory
	GetTaskDependencies(dir string, tasks []graph.Task, buildContext *BuildContext) []graph.Task
	
	// GetRootDir returns the root directory of this compilation root
	GetRootDir() string
	
	// GetType returns the type of compilation root (e.g., "gradle", "maven", etc.)
	GetType() string
	
	// ResolveProjectDependencies resolves dependencies between compilation roots
	ResolveProjectDependencies(buildGraph *graph.Graph, allRoots []CompilationRoot) error
}

// StructureDiscoverer discovers compilation roots in the file system
type StructureDiscoverer interface {
	// IsCompilationRoot checks if the given directory is a compilation root
	// Returns a CompilationRoot instance if it is, nil otherwise
	IsCompilationRoot(ctx context.Context, dir string) (CompilationRoot, error)
	
	// Name returns the name of this structure discoverer
	Name() string
}