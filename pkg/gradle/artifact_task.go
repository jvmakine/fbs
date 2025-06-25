package gradle

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/graph"
)

// ArtifactDownload represents a task that downloads an external artifact
type ArtifactDownload struct {
	group       string
	name        string
	version     string
	artifact    string // full coordinate like "group:name:version"
	localPath   string // path in local gradle cache
	id          string
	hash        string
}

// NewArtifactDownload creates a new artifact download task
func NewArtifactDownload(group, name, version string) *ArtifactDownload {
	task := &ArtifactDownload{
		group:    group,
		name:     name,
		version:  version,
		artifact: fmt.Sprintf("%s:%s:%s", group, name, version),
	}
	
	// Generate local cache path (simplified gradle cache structure)
	homeDir, _ := os.UserHomeDir()
	task.localPath = filepath.Join(homeDir, ".gradle", "caches", "modules-2", "files-2.1", 
		group, name, version, name+"-"+version+".jar")
	
	// Generate ID and hash
	task.id = task.generateID()
	task.hash = task.generateHash()
	
	return task
}

// ID returns the unique identifier for this task
func (a *ArtifactDownload) ID() string {
	return a.id
}

// Name returns the human-readable name of this task
func (a *ArtifactDownload) Name() string {
	return "artifact-download"
}

// Hash returns a hash representing the task's configuration
func (a *ArtifactDownload) Hash() string {
	return a.hash
}

// Dependencies returns the list of tasks this task depends on (none for external artifacts)
func (a *ArtifactDownload) Dependencies() []graph.Task {
	return []graph.Task{}
}

// AddDependency adds a dependency to this task (not applicable for external artifacts)
func (a *ArtifactDownload) AddDependency(task graph.Task) {
	// External artifacts don't have dependencies in our model
}

// Directory returns the directory this task operates in (gradle cache)
func (a *ArtifactDownload) Directory() string {
	return filepath.Dir(a.localPath)
}

// TaskType returns the type of this task
func (a *ArtifactDownload) TaskType() graph.TaskType {
	return graph.TaskTypeDeps
}

// Execute runs the artifact download task
func (a *ArtifactDownload) Execute(ctx context.Context, workDir string, dependencyInputs []graph.DependencyInput) graph.TaskResult {
	// Check if artifact already exists
	if _, err := os.Stat(a.localPath); err == nil {
		// Return the existing artifact file
		relPath, err := filepath.Rel(workDir, a.localPath)
		if err != nil {
			relPath = a.localPath
		}
		return graph.TaskResult{
			Files: []string{relPath},
		}
	}
	
	// Create cache directory
	if err := os.MkdirAll(filepath.Dir(a.localPath), 0755); err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to create cache directory: %w", err),
		}
	}
	
	// Construct download URL (using Maven Central as default)
	downloadURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.jar",
		strings.ReplaceAll(a.group, ".", "/"), a.name, a.version, a.name, a.version)
	
	// Download the artifact
	resp, err := http.Get(downloadURL)
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to download artifact %s: %w", a.artifact, err),
		}
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to download artifact %s: HTTP %d", a.artifact, resp.StatusCode),
		}
	}
	
	// Create the local file
	file, err := os.Create(a.localPath)
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to create local file: %w", err),
		}
	}
	defer file.Close()
	
	// Copy the content
	_, err = io.Copy(file, resp.Body)
	if err != nil {
		return graph.TaskResult{
			Error: fmt.Errorf("failed to save artifact: %w", err),
		}
	}
	
	// Return the downloaded artifact file
	relPath, err := filepath.Rel(workDir, a.localPath)
	if err != nil {
		relPath = a.localPath
	}
	
	return graph.TaskResult{
		Files: []string{relPath},
	}
}

// GetArtifact returns the artifact coordinate
func (a *ArtifactDownload) GetArtifact() string {
	return a.artifact
}

// GetLocalPath returns the local cache path
func (a *ArtifactDownload) GetLocalPath() string {
	return a.localPath
}

// GetGroup returns the group ID
func (a *ArtifactDownload) GetGroup() string {
	return a.group
}

// GetName returns the artifact name
func (a *ArtifactDownload) GetName() string {
	return a.name
}

// GetVersion returns the version
func (a *ArtifactDownload) GetVersion() string {
	return a.version
}

// generateID creates a unique ID for this task
func (a *ArtifactDownload) generateID() string {
	hasher := sha256.New()
	hasher.Write([]byte("artifact-download"))
	hasher.Write([]byte(a.artifact))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// generateHash creates a hash for this task's configuration
func (a *ArtifactDownload) generateHash() string {
	hasher := sha256.New()
	hasher.Write([]byte(a.artifact))
	hasher.Write([]byte(a.localPath))
	return fmt.Sprintf("%x", hasher.Sum(nil))
}