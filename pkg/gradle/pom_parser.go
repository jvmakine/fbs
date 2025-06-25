package gradle

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// MavenPOM represents a Maven POM file structure
type MavenPOM struct {
	XMLName      xml.Name     `xml:"project"`
	GroupID      string       `xml:"groupId"`
	ArtifactID   string       `xml:"artifactId"`
	Version      string       `xml:"version"`
	Dependencies Dependencies `xml:"dependencies"`
	Properties   Properties   `xml:"properties"`
}

// Dependencies represents the dependencies section of a POM
type Dependencies struct {
	Dependency []Dependency `xml:"dependency"`
}

// Dependency represents a single dependency in a POM
type Dependency struct {
	GroupID    string `xml:"groupId"`
	ArtifactID string `xml:"artifactId"`
	Version    string `xml:"version"`
	Scope      string `xml:"scope"`
	Optional   string `xml:"optional"`
}

// Properties represents the properties section of a POM
type Properties struct {
	JunitVersion string `xml:"junit-jupiter.version"`
	// Add more properties as needed
}

// MavenArtifact represents a resolved Maven artifact
type MavenArtifact struct {
	GroupID    string
	ArtifactID string
	Version    string
}

// String returns the Maven coordinate string
func (a *MavenArtifact) String() string {
	return fmt.Sprintf("%s:%s:%s", a.GroupID, a.ArtifactID, a.Version)
}

// DownloadPOM downloads and parses a POM file from Maven Central
func DownloadPOM(groupId, artifactId, version string) (*MavenPOM, error) {
	// Construct POM download URL
	pomURL := fmt.Sprintf("https://repo1.maven.org/maven2/%s/%s/%s/%s-%s.pom",
		strings.ReplaceAll(groupId, ".", "/"), artifactId, version, artifactId, version)
	
	// Download the POM
	resp, err := http.Get(pomURL)
	if err != nil {
		return nil, fmt.Errorf("failed to download POM for %s:%s:%s: %w", groupId, artifactId, version, err)
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to download POM for %s:%s:%s: HTTP %d", groupId, artifactId, version, resp.StatusCode)
	}
	
	// Parse the POM
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read POM content: %w", err)
	}
	
	var pom MavenPOM
	if err := xml.Unmarshal(body, &pom); err != nil {
		return nil, fmt.Errorf("failed to parse POM XML: %w", err)
	}
	
	// Fill in inherited values if empty
	if pom.GroupID == "" {
		pom.GroupID = groupId
	}
	if pom.ArtifactID == "" {
		pom.ArtifactID = artifactId
	}
	if pom.Version == "" {
		pom.Version = version
	}
	
	return &pom, nil
}

// GetTransitiveDependencies resolves transitive dependencies for an artifact
func GetTransitiveDependencies(groupId, artifactId, version string, visited map[string]bool) ([]*MavenArtifact, error) {
	key := fmt.Sprintf("%s:%s:%s", groupId, artifactId, version)
	
	// Avoid circular dependencies
	if visited[key] {
		return nil, nil
	}
	visited[key] = true
	
	pom, err := DownloadPOM(groupId, artifactId, version)
	if err != nil {
		// If we can't download the POM, just return empty (might be a JAR-only artifact)
		return nil, nil
	}
	
	var result []*MavenArtifact
	
	for _, dep := range pom.Dependencies.Dependency {
		// Skip test and provided scope dependencies
		if dep.Scope == "test" || dep.Scope == "provided" {
			continue
		}
		
		// Skip optional dependencies
		if dep.Optional == "true" {
			continue
		}
		
		// Resolve version if needed (simplified - just use the declared version)
		version := dep.Version
		if version == "" {
			// For now, skip dependencies without explicit versions
			continue
		}
		
		// Add this dependency
		artifact := &MavenArtifact{
			GroupID:    dep.GroupID,
			ArtifactID: dep.ArtifactID,
			Version:    version,
		}
		result = append(result, artifact)
		
		// Recursively get transitive dependencies
		transitives, err := GetTransitiveDependencies(dep.GroupID, dep.ArtifactID, version, visited)
		if err != nil {
			// Log error but continue with other dependencies
			continue
		}
		result = append(result, transitives...)
	}
	
	return result, nil
}