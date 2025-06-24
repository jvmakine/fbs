package discoverer

import (
	"context"

	"fbs/pkg/graph"
)

// DiscoveryResult represents the result of discovering tasks from a path
type DiscoveryResult struct {
	// Tasks contains the discovered tasks
	Tasks []graph.Task
	// Errors contains any errors encountered during discovery
	Errors []error
	// Path is the path that was scanned
	Path string
}

// Discoverer is an interface for discovering build tasks from filesystem paths
type Discoverer interface {
	// Discover finds and returns build tasks from the given path
	// The path can be a directory or a specific file
	// potentialDependencies contains tasks discovered from subdirectories that could be dependencies
	// buildContext contains metadata from parent directories and context discoverers
	// Returns a DiscoveryResult containing the found tasks and any errors
	Discover(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *BuildContext) (*DiscoveryResult, error)

	// Name returns a human-readable name for this discoverer
	Name() string
}

// MultiDiscoverer combines multiple discoverers and tries them in order
type MultiDiscoverer struct {
	discoverers []Discoverer
}

// NewMultiDiscoverer creates a new MultiDiscoverer with the given discoverers
func NewMultiDiscoverer(discoverers ...Discoverer) *MultiDiscoverer {
	return &MultiDiscoverer{
		discoverers: discoverers,
	}
}

// Discover tries each discoverer in order and combines all results
func (m *MultiDiscoverer) Discover(ctx context.Context, path string, potentialDependencies []graph.Task, buildContext *BuildContext) (*DiscoveryResult, error) {
	var allTasks []graph.Task
	var allErrors []error
	
	for _, discoverer := range m.discoverers {
		result, err := discoverer.Discover(ctx, path, potentialDependencies, buildContext)
		if err != nil {
			allErrors = append(allErrors, err)
			continue
		}
		
		// Combine tasks from all discoverers
		allTasks = append(allTasks, result.Tasks...)
		allErrors = append(allErrors, result.Errors...)
	}
	
	return &DiscoveryResult{
		Tasks:  allTasks,
		Errors: allErrors,
		Path:   path,
	}, nil
}

// Name returns the name of the MultiDiscoverer
func (m *MultiDiscoverer) Name() string {
	return "MultiDiscoverer"
}

// AddDiscoverer adds a new discoverer to the chain
func (m *MultiDiscoverer) AddDiscoverer(discoverer Discoverer) {
	m.discoverers = append(m.discoverers, discoverer)
}

// GetDiscoverers returns all registered discoverers
func (m *MultiDiscoverer) GetDiscoverers() []Discoverer {
	return m.discoverers
}