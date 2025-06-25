package gradle

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// GradleDependency represents a dependency from a Gradle build file
type GradleDependency struct {
	Type    string // "implementation", "testImplementation", etc.
	Group   string
	Name    string
	Version string
	IsLocal bool // true for project dependencies
	Raw     string // original dependency string
}

// GradleBuildInfo contains parsed information from a Gradle build file
type GradleBuildInfo struct {
	ProjectDir   string
	Dependencies []GradleDependency
	Plugins      []string
}

// ParseGradleBuildFile parses a build.gradle.kts file and extracts dependency information
func ParseGradleBuildFile(buildFilePath string) (*GradleBuildInfo, error) {
	file, err := os.Open(buildFilePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	
	buildInfo := &GradleBuildInfo{
		ProjectDir:   filepath.Dir(buildFilePath),
		Dependencies: []GradleDependency{},
		Plugins:      []string{},
	}
	
	scanner := bufio.NewScanner(file)
	inDependenciesBlock := false
	inPluginsBlock := false
	
	// Regular expressions for parsing
	dependencyRegex := regexp.MustCompile(`^\s*(implementation|testImplementation|api|compileOnly|runtimeOnly)\s*\(\s*(.+)\s*\)`)
	projectDependencyRegex := regexp.MustCompile(`project\s*\(\s*["']([^"']+)["']\s*\)`)
	stringDependencyRegex := regexp.MustCompile(`["']([^"']+)["']`)
	libsDependencyRegex := regexp.MustCompile(`libs\.([^)]+)`)
	pluginRegex := regexp.MustCompile(`^\s*(id|kotlin)\s*\(\s*["']([^"']+)["']\s*\)`)
	
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		
		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		
		// Track if we're in dependencies or plugins block
		if strings.Contains(line, "dependencies {") {
			inDependenciesBlock = true
			continue
		}
		if strings.Contains(line, "plugins {") {
			inPluginsBlock = true
			continue
		}
		if line == "}" {
			inDependenciesBlock = false
			inPluginsBlock = false
			continue
		}
		
		// Parse dependencies
		if inDependenciesBlock {
			if matches := dependencyRegex.FindStringSubmatch(line); matches != nil {
				depType := matches[1]
				depString := matches[2]
				
				dependency := GradleDependency{
					Type: depType,
					Raw:  depString,
				}
				
				// Check if it's a project dependency
				if projectMatches := projectDependencyRegex.FindStringSubmatch(depString); projectMatches != nil {
					dependency.IsLocal = true
					dependency.Name = projectMatches[1]
				} else if libsMatches := libsDependencyRegex.FindStringSubmatch(depString); libsMatches != nil {
					// Handle libs.xyz version catalog references
					libraryRef := libsMatches[1]
					dependency.Name = libraryRef // Store the version catalog reference
					// The actual resolution will happen later when we have access to the version catalog
				} else {
					// Parse external dependency
					if stringMatches := stringDependencyRegex.FindStringSubmatch(depString); stringMatches != nil {
						parts := strings.Split(stringMatches[1], ":")
						if len(parts) >= 2 {
							dependency.Group = parts[0]
							dependency.Name = parts[1]
							if len(parts) >= 3 {
								dependency.Version = parts[2]
							}
						}
					}
				}
				
				buildInfo.Dependencies = append(buildInfo.Dependencies, dependency)
			}
		}
		
		// Parse plugins
		if inPluginsBlock {
			if matches := pluginRegex.FindStringSubmatch(line); matches != nil {
				pluginId := matches[2]
				buildInfo.Plugins = append(buildInfo.Plugins, pluginId)
			}
		}
	}
	
	return buildInfo, scanner.Err()
}

// GetExternalDependencies returns only external (non-project) dependencies
func (b *GradleBuildInfo) GetExternalDependencies() []GradleDependency {
	var external []GradleDependency
	for _, dep := range b.Dependencies {
		if !dep.IsLocal {
			external = append(external, dep)
		}
	}
	return external
}

// GetProjectDependencies returns only project dependencies
func (b *GradleBuildInfo) GetProjectDependencies() []GradleDependency {
	var projects []GradleDependency
	for _, dep := range b.Dependencies {
		if dep.IsLocal {
			projects = append(projects, dep)
		}
	}
	return projects
}

// HasPlugin checks if a specific plugin is configured
func (b *GradleBuildInfo) HasPlugin(pluginId string) bool {
	for _, plugin := range b.Plugins {
		if plugin == pluginId {
			return true
		}
	}
	return false
}