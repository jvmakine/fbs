package gradle

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/discoverer"
)

// GradleArtefactVersions contains version information from Gradle version catalogs
type GradleArtefactVersions struct {
	// Versions maps version reference names to version strings
	Versions map[string]string
	// Libraries maps library reference names to their coordinates
	Libraries map[string]LibraryCoordinate
	// Plugins maps plugin reference names to their information
	Plugins map[string]PluginCoordinate
	// ProjectDir is the directory where this version catalog was found
	ProjectDir string
}

// LibraryCoordinate represents a library dependency coordinate
type LibraryCoordinate struct {
	Group    string
	Name     string
	Version  string
	Module   string // full module coordinate like "group:name"
}

// PluginCoordinate represents a plugin coordinate
type PluginCoordinate struct {
	ID      string
	Version string
}

// NewGradleArtefactVersions creates a new empty GradleArtefactVersions
func NewGradleArtefactVersions(projectDir string) *GradleArtefactVersions {
	return &GradleArtefactVersions{
		Versions:   make(map[string]string),
		Libraries:  make(map[string]LibraryCoordinate),
		Plugins:    make(map[string]PluginCoordinate),
		ProjectDir: projectDir,
	}
}

// GetLibraryVersion returns the version for a library, resolving version references
func (gav *GradleArtefactVersions) GetLibraryVersion(libraryName string) string {
	if lib, exists := gav.Libraries[libraryName]; exists {
		if lib.Version != "" {
			return lib.Version
		}
	}
	return ""
}

// GetVersion returns a version by reference name
func (gav *GradleArtefactVersions) GetVersion(versionRef string) string {
	return gav.Versions[versionRef]
}

// GetLibrary returns a library coordinate by reference name
func (gav *GradleArtefactVersions) GetLibrary(libraryRef string) (LibraryCoordinate, bool) {
	lib, exists := gav.Libraries[libraryRef]
	return lib, exists
}

// GetPlugin returns a plugin coordinate by reference name
func (gav *GradleArtefactVersions) GetPlugin(pluginRef string) (PluginCoordinate, bool) {
	plugin, exists := gav.Plugins[pluginRef]
	return plugin, exists
}

// GradleContextDiscoverer discovers Gradle version catalog information
type GradleContextDiscoverer struct{}

// NewGradleContextDiscoverer creates a new Gradle context discoverer
func NewGradleContextDiscoverer() *GradleContextDiscoverer {
	return &GradleContextDiscoverer{}
}

// Name returns the name of this context discoverer
func (d *GradleContextDiscoverer) Name() string {
	return "GradleContextDiscoverer"
}

// DiscoverContext examines a directory for Gradle version catalogs and adds them to BuildContext
func (d *GradleContextDiscoverer) DiscoverContext(ctx context.Context, path string, buildContext *discoverer.BuildContext) error {
	// Check if path exists and is a directory
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return nil // Not an error, just no context to discover
	}

	// Look for gradle/libs.versions.toml file
	versionCatalogPath := filepath.Join(path, "gradle", "libs.versions.toml")
	if _, err := os.Stat(versionCatalogPath); err != nil {
		return nil // No version catalog found
	}

	// Parse the version catalog
	versions, err := d.parseVersionCatalog(versionCatalogPath)
	if err != nil {
		return fmt.Errorf("failed to parse version catalog at %s: %w", versionCatalogPath, err)
	}

	versions.ProjectDir = path
	buildContext.Set(versions)
	return nil
}

// parseVersionCatalog parses a Gradle version catalog TOML file
func (d *GradleContextDiscoverer) parseVersionCatalog(filePath string) (*GradleArtefactVersions, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read version catalog: %w", err)
	}

	versions := NewGradleArtefactVersions("")
	
	// Simple TOML parser for the specific structure we expect
	lines := strings.Split(string(content), "\n")
	currentSection := ""
	
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		
		// Check for section headers
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.Trim(line, "[]")
			continue
		}
		
		// Parse key-value pairs based on current section
		if strings.Contains(line, "=") {
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				continue
			}
			
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			value = strings.Trim(value, "\"")
			
			switch currentSection {
			case "versions":
				versions.Versions[key] = value
			case "libraries":
				if lib := d.parseLibraryCoordinate(value); lib != nil {
					versions.Libraries[key] = *lib
				}
			case "plugins":
				if plugin := d.parsePluginCoordinate(value); plugin != nil {
					versions.Plugins[key] = *plugin
				}
			}
		}
	}
	
	// Resolve version references in libraries
	d.resolveVersionReferences(versions)
	
	return versions, nil
}

// parseLibraryCoordinate parses a library coordinate string
func (d *GradleContextDiscoverer) parseLibraryCoordinate(value string) *LibraryCoordinate {
	// Handle both formats:
	// { module = "group:name", version.ref = "version-ref" }
	// { module = "group:name", version = "1.0.0" }
	// "group:name:version"
	
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		// Parse object format
		return d.parseLibraryObject(value)
	}
	
	// Parse simple string format "group:name:version"
	parts := strings.Split(value, ":")
	if len(parts) >= 2 {
		lib := &LibraryCoordinate{
			Group:  parts[0],
			Name:   parts[1],
			Module: parts[0] + ":" + parts[1],
		}
		if len(parts) >= 3 {
			lib.Version = parts[2]
		}
		return lib
	}
	
	return nil
}

// parseLibraryObject parses library object format
func (d *GradleContextDiscoverer) parseLibraryObject(value string) *LibraryCoordinate {
	lib := &LibraryCoordinate{}
	
	// Remove braces and split by comma
	content := strings.Trim(value, "{}")
	parts := strings.Split(content, ",")
	
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if strings.Contains(part, "=") {
			keyValue := strings.SplitN(part, "=", 2)
			if len(keyValue) != 2 {
				continue
			}
			
			key := strings.TrimSpace(keyValue[0])
			val := strings.TrimSpace(keyValue[1])
			val = strings.Trim(val, "\"")
			
			switch key {
			case "module":
				lib.Module = val
				// Split module into group and name
				if moduleParts := strings.Split(val, ":"); len(moduleParts) >= 2 {
					lib.Group = moduleParts[0]
					lib.Name = moduleParts[1]
				}
			case "version":
				lib.Version = val
			case "version.ref":
				lib.Version = "$" + val // Mark as reference for later resolution
			}
		}
	}
	
	return lib
}

// parsePluginCoordinate parses a plugin coordinate
func (d *GradleContextDiscoverer) parsePluginCoordinate(value string) *PluginCoordinate {
	if strings.HasPrefix(value, "{") && strings.HasSuffix(value, "}") {
		// Parse object format
		plugin := &PluginCoordinate{}
		content := strings.Trim(value, "{}")
		parts := strings.Split(content, ",")
		
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.Contains(part, "=") {
				keyValue := strings.SplitN(part, "=", 2)
				if len(keyValue) != 2 {
					continue
				}
				
				key := strings.TrimSpace(keyValue[0])
				val := strings.TrimSpace(keyValue[1])
				val = strings.Trim(val, "\"")
				
				switch key {
				case "id":
					plugin.ID = val
				case "version":
					plugin.Version = val
				case "version.ref":
					plugin.Version = "$" + val // Mark as reference
				}
			}
		}
		
		return plugin
	}
	
	return nil
}

// resolveVersionReferences resolves version references in libraries and plugins
func (d *GradleContextDiscoverer) resolveVersionReferences(versions *GradleArtefactVersions) {
	// Resolve library version references
	for key, lib := range versions.Libraries {
		if strings.HasPrefix(lib.Version, "$") {
			ref := strings.TrimPrefix(lib.Version, "$")
			if resolvedVersion, exists := versions.Versions[ref]; exists {
				lib.Version = resolvedVersion
				versions.Libraries[key] = lib
			}
		}
	}
	
	// Resolve plugin version references
	for key, plugin := range versions.Plugins {
		if strings.HasPrefix(plugin.Version, "$") {
			ref := strings.TrimPrefix(plugin.Version, "$")
			if resolvedVersion, exists := versions.Versions[ref]; exists {
				plugin.Version = resolvedVersion
				versions.Plugins[key] = plugin
			}
		}
	}
}