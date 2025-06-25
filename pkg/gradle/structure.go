package gradle

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"fbs/pkg/config"
	"fbs/pkg/discoverer"
	"fbs/pkg/graph"
	"fbs/pkg/kotlin"
)

// GradleStructureDiscoverer discovers Gradle compilation roots
type GradleStructureDiscoverer struct{
	cache map[string]*GradleCompilationRoot // Cache compilation roots by directory
}

// NewGradleStructureDiscoverer creates a new Gradle structure discoverer
func NewGradleStructureDiscoverer() *GradleStructureDiscoverer {
	return &GradleStructureDiscoverer{
		cache: make(map[string]*GradleCompilationRoot),
	}
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
	
	// Check cache first
	if cached, exists := d.cache[dir]; exists {
		return cached, nil
	}
	
	// This is a Gradle compilation root, create and cache it
	root := NewGradleCompilationRoot(dir)
	d.cache[dir] = root
	return root, nil
}

// GradleCompilationRoot represents a Gradle project compilation root
type GradleCompilationRoot struct {
	rootDir          string
	versions         *GradleArtefactVersions
	buildInfo        *GradleBuildInfo
	jarTask          *JarCompile         // Cached JAR task
	artifactTasks    []*ArtifactDownload // Cached artifact tasks
	jarTaskReturned  bool                // Track if JAR task has been returned
	artifactsReturned bool               // Track if artifact tasks have been returned
}

// NewGradleCompilationRoot creates a new Gradle compilation root
func NewGradleCompilationRoot(rootDir string) *GradleCompilationRoot {
	root := &GradleCompilationRoot{
		rootDir: rootDir,
	}
	
	// Try to load version catalog from the project root
	root.loadVersionCatalog()
	
	// Try to parse build file
	root.loadBuildInfo()
	
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
func (g *GradleCompilationRoot) GetTaskDependencies(dir string, tasks []graph.Task, buildContext *discoverer.BuildContext) []graph.Task {
	var allTasks []graph.Task
	
	// Get repository configuration from BuildContext
	var repositories []string
	if buildContext != nil {
		if configObj := buildContext.GetByExample((*config.Config)(nil)); configObj != nil {
			cfg := configObj.(*config.Config)
			var artifactConfig config.ArtifactDownloadConfig
			if err := cfg.GetDiscovererConfig("artifact-download", &artifactConfig); err == nil {
				repositories = artifactConfig.Repositories
			}
		}
	}
	
	// Separate different types of tasks
	var kotlinCompileTasks []*kotlin.KotlinCompile
	var junitTestTasks []*kotlin.JunitTest
	var mainKotlinTasks []*kotlin.KotlinCompile
	var testKotlinTasks []*kotlin.KotlinCompile
	
	for _, task := range tasks {
		switch t := task.(type) {
		case *kotlin.KotlinCompile:
			kotlinCompileTasks = append(kotlinCompileTasks, t)
			// Check if this is a main source compile task
			if strings.Contains(t.GetSourceDir(), "src/main") {
				mainKotlinTasks = append(mainKotlinTasks, t)
			}
			// Check if this is a test source compile task
			if strings.Contains(t.GetSourceDir(), "src/test") {
				testKotlinTasks = append(testKotlinTasks, t)
			}
		case *kotlin.JunitTest:
			junitTestTasks = append(junitTestTasks, t)
		}
		allTasks = append(allTasks, task)
	}
	
	// 1. Create or reuse JAR compilation task for main sources
	if len(mainKotlinTasks) > 0 && g.jarTask == nil {
		// Create JAR task only once per compilation root
		g.jarTask = NewJarCompile(g.rootDir, []string{}) // Start with empty sources
	}
	
	// Add main kotlin tasks as dependencies to JAR task if it exists
	if g.jarTask != nil {
		for _, kotlinTask := range mainKotlinTasks {
			g.jarTask.AddDependency(kotlinTask)
		}
		// Always include JAR task when there are main tasks (first time) or test tasks that need it
		if len(mainKotlinTasks) > 0 && !g.jarTaskReturned {
			allTasks = append(allTasks, g.jarTask)
			g.jarTaskReturned = true
		} else if len(testKotlinTasks) > 0 {
			// Also include the JAR task when we have test tasks that depend on it
			allTasks = append(allTasks, g.jarTask)
		}
	}
	
	// 2. Create external artifact download tasks (once per compilation root)
	if len(g.artifactTasks) == 0 && g.buildInfo != nil {
		for _, dep := range g.buildInfo.GetExternalDependencies() {
			var group, name, version string
			
			// Check if this is a version catalog reference
			if dep.Group == "" && dep.Name != "" && g.versions != nil {
				// This is a libs.xyz reference, resolve it
				// Try with the exact name first
				if lib, exists := g.versions.GetLibrary(dep.Name); exists {
					group = lib.Group
					name = lib.Name
					version = lib.Version
				} else {
					// Try converting dots to hyphens (common gradle convention)
					hyphenatedName := strings.ReplaceAll(dep.Name, ".", "-")
					if lib, exists := g.versions.GetLibrary(hyphenatedName); exists {
						group = lib.Group
						name = lib.Name
						version = lib.Version
					}
				}
			} else if dep.Group != "" && dep.Name != "" {
				// This is a direct dependency
				group = dep.Group
				name = dep.Name
				version = dep.Version
				// Resolve version from version catalog if needed
				if version == "" && g.versions != nil {
					version = g.versions.GetLibraryVersion(dep.Group + "-" + dep.Name)
				}
			}
			
			if group != "" && name != "" && version != "" {
				artifactTask := NewArtifactDownload(group, name, version, repositories)
				g.artifactTasks = append(g.artifactTasks, artifactTask)
			}
		}
	}
	
	// Add artifact tasks to results (they're shared across all directories, but only once)
	if len(g.artifactTasks) > 0 && !g.artifactsReturned {
		for _, artifactTask := range g.artifactTasks {
			allTasks = append(allTasks, artifactTask)
		}
		g.artifactsReturned = true
	}
	
	// 3. Add external dependencies to all compilation tasks
	for _, kotlinTask := range kotlinCompileTasks {
		for _, artifactTask := range g.artifactTasks {
			kotlinTask.AddDependency(artifactTask)
		}
	}
	
	// 3.5. Add JAR compilation as dependency to test compilation tasks
	// This must happen after the JAR task is created and added to allTasks
	if g.jarTask != nil {
		for _, kotlinTask := range kotlinCompileTasks {
			if strings.Contains(kotlinTask.GetSourceDir(), "src/test") {
				kotlinTask.AddDependency(g.jarTask)
			}
		}
	}
	
	// 4. Add JAR task as dependency for test tasks (if it exists)
	if g.jarTask != nil {
		for _, junitTask := range junitTestTasks {
			junitTask.AddDependency(g.jarTask)
		}
	}
	
	// 5. Add JUnit Console Launcher for test execution (if we have JUnit tests)
	if len(junitTestTasks) > 0 {
		// Create console launcher artifact task if not already created
		var consoleLauncherTask *ArtifactDownload
		
		// Check if we already have a console launcher task
		found := false
		for _, task := range g.artifactTasks {
			if task.GetName() == "junit-platform-console-standalone" {
				consoleLauncherTask = task
				found = true
				break
			}
		}
		
		if !found {
			consoleLauncherTask = NewArtifactDownload("org.junit.platform", "junit-platform-console-standalone", "1.10.0", repositories)
			g.artifactTasks = append(g.artifactTasks, consoleLauncherTask)
			
			// Add to results if artifacts haven't been returned yet
			if !g.artifactsReturned {
				allTasks = append(allTasks, consoleLauncherTask)
			}
		}
		
		// Add console launcher as dependency to all JUnit test tasks
		for _, junitTask := range junitTestTasks {
			junitTask.AddDependency(consoleLauncherTask)
		}
	}
	
	// 6. Inject kotlin compile tasks as dependencies of junit test tasks
	for _, junitTask := range junitTestTasks {
		for _, kotlinTask := range kotlinCompileTasks {
			// Check if this dependency doesn't already exist
			if !g.hasDependency(junitTask, kotlinTask) {
				junitTask.AddDependency(kotlinTask)
			}
		}
	}
	
	return allTasks
}

// loadVersionCatalog loads the Gradle version catalog if it exists
func (g *GradleCompilationRoot) loadVersionCatalog() {
	// Search upward from the compilation root to find version catalog
	currentDir := g.rootDir
	
	for {
		versionCatalogPath := filepath.Join(currentDir, "gradle", "libs.versions.toml")
		if _, err := os.Stat(versionCatalogPath); err == nil {
			// Found version catalog, parse it
			contextDiscoverer := NewGradleContextDiscoverer()
			versions, err := contextDiscoverer.ParseVersionCatalog(versionCatalogPath)
			if err != nil {
				return // Failed to parse, continue without versions
			}
			
			versions.ProjectDir = g.rootDir
			g.versions = versions
			return
		}
		
		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// Reached filesystem root
			break
		}
		currentDir = parentDir
	}
}

// loadBuildInfo loads and parses the build.gradle.kts file
func (g *GradleCompilationRoot) loadBuildInfo() {
	buildFilePath := filepath.Join(g.rootDir, "build.gradle.kts")
	if _, err := os.Stat(buildFilePath); err != nil {
		return // No build file found
	}
	
	buildInfo, err := ParseGradleBuildFile(buildFilePath)
	if err != nil {
		return // Failed to parse, continue without build info
	}
	
	g.buildInfo = buildInfo
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

// GetBuildInfo returns the parsed build information for this compilation root
func (g *GradleCompilationRoot) GetBuildInfo() *GradleBuildInfo {
	return g.buildInfo
}

// ResolveProjectDependencies resolves dependencies between compilation roots
func (g *GradleCompilationRoot) ResolveProjectDependencies(buildGraph *graph.Graph, allRoots []discoverer.CompilationRoot) error {
	if g.buildInfo == nil {
		return nil // No build info to process
	}
	
	// First, create a map of project paths to JAR tasks
	projectPathToJarTask := make(map[string]graph.Task)
	
	// Collect all JAR tasks and their associated project paths
	for _, task := range buildGraph.GetTasks() {
		if task.Name() == "jar-compile" {
			// Find the compilation root for this task
			taskDir := task.Directory()
			for _, root := range allRoots {
				if root.GetRootDir() == taskDir {
					// Get the project path for this compilation root
					projectPath := getProjectPathFromRoot(root)
					if projectPath != "" {
						projectPathToJarTask[projectPath] = task
					}
					break
				}
			}
		}
	}
	
	// Find the JAR task for this compilation root
	var currentJarTask graph.Task
	for _, task := range buildGraph.GetTasks() {
		if task.Name() == "jar-compile" && task.Directory() == g.rootDir {
			currentJarTask = task
			break
		}
	}
	
	if currentJarTask != nil {
		// Cast to JarCompile to add dependencies
		if jarTask, ok := currentJarTask.(*JarCompile); ok {
			// Add dependencies for each project dependency
			for _, dep := range g.buildInfo.GetProjectDependencies() {
				dependencyJarTask := projectPathToJarTask[dep.Name]
				if dependencyJarTask != nil {
					jarTask.AddDependency(dependencyJarTask)
				}
			}
		}
	}
	
	return nil
}

// getProjectPathFromRoot extracts the Gradle project path from a compilation root
func getProjectPathFromRoot(root discoverer.CompilationRoot) string {
	// For a compilation root like "/path/to/cash-server/login-audit/service"
	// We want to extract ":login-audit:service"
	rootDir := root.GetRootDir()
	
	// Find the topmost directory with a settings.gradle or build.gradle
	current := rootDir
	var projectRoot string
	
	for {
		parent := filepath.Dir(current)
		if parent == current || parent == "/" {
			break
		}
		
		// Check if parent has settings.gradle (indicates project root)
		if _, err := os.Stat(filepath.Join(parent, "settings.gradle")); err == nil {
			projectRoot = parent
			break
		}
		if _, err := os.Stat(filepath.Join(parent, "settings.gradle.kts")); err == nil {
			projectRoot = parent
			break
		}
		
		current = parent
	}
	
	if projectRoot == "" {
		return ""
	}
	
	// Calculate relative path from project root
	relPath, err := filepath.Rel(projectRoot, rootDir)
	if err != nil {
		return ""
	}
	
	// Convert filesystem path to Gradle project path
	// "login-audit/service" -> ":login-audit:service"
	projectPath := ":" + strings.ReplaceAll(relPath, "/", ":")
	return projectPath
}