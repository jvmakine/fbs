package gradle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGradleStructureDiscoverer_IsCompilationRoot(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "gradle_structure_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	discoverer := NewGradleStructureDiscoverer()

	// Test directory without build.gradle.kts
	ctx := context.Background()
	root, err := discoverer.IsCompilationRoot(ctx, tempDir)
	if err != nil {
		t.Fatalf("IsCompilationRoot failed: %v", err)
	}
	if root != nil {
		t.Error("Expected nil for directory without build.gradle.kts")
	}

	// Create build.gradle.kts file
	buildFile := filepath.Join(tempDir, "build.gradle.kts")
	err = os.WriteFile(buildFile, []byte("// test build file"), 0644)
	if err != nil {
		t.Fatalf("Failed to create build.gradle.kts: %v", err)
	}

	// Test directory with build.gradle.kts
	root, err = discoverer.IsCompilationRoot(ctx, tempDir)
	if err != nil {
		t.Fatalf("IsCompilationRoot failed: %v", err)
	}
	if root == nil {
		t.Fatal("Expected non-nil for directory with build.gradle.kts")
	}

	// Verify root properties
	if root.GetType() != "gradle" {
		t.Errorf("Expected type 'gradle', got '%s'", root.GetType())
	}

	resolvedRootDir, _ := filepath.EvalSymlinks(root.GetRootDir())
	resolvedTempDir, _ := filepath.EvalSymlinks(tempDir)
	if resolvedRootDir != resolvedTempDir {
		t.Errorf("Expected root dir %s, got %s", resolvedTempDir, resolvedRootDir)
	}
}

func TestGradleCompilationRoot_GetBuildContext(t *testing.T) {
	// Create temporary directory
	tempDir, err := os.MkdirTemp("", "gradle_compilation_root_test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	// Create Gradle version catalog
	gradleDir := filepath.Join(tempDir, "gradle")
	err = os.MkdirAll(gradleDir, 0755)
	if err != nil {
		t.Fatalf("Failed to create gradle dir: %v", err)
	}

	versionCatalog := filepath.Join(gradleDir, "libs.versions.toml")
	catalogContent := `[versions]
kotlin = "1.9.0"

[libraries]
kotlin-stdlib = { module = "org.jetbrains.kotlin:kotlin-stdlib", version.ref = "kotlin" }

[plugins]
kotlin-jvm = { id = "org.jetbrains.kotlin.jvm", version.ref = "kotlin" }
`
	err = os.WriteFile(versionCatalog, []byte(catalogContent), 0644)
	if err != nil {
		t.Fatalf("Failed to create version catalog: %v", err)
	}

	// Create compilation root
	root := NewGradleCompilationRoot(tempDir)

	// Test GetBuildContext
	buildContext := root.GetBuildContext("some/dir")
	if buildContext == nil {
		t.Fatal("Expected non-nil build context")
	}

	// Verify that version catalog was loaded
	versions := buildContext.GetByExample((*GradleArtefactVersions)(nil))
	if versions == nil {
		t.Fatal("Expected GradleArtefactVersions in build context")
	}

	gradleVersions := versions.(*GradleArtefactVersions)
	if gradleVersions.GetVersion("kotlin") != "1.9.0" {
		t.Errorf("Expected kotlin version '1.9.0', got '%s'", gradleVersions.GetVersion("kotlin"))
	}
}

func TestGradleStructureDiscoverer_Name(t *testing.T) {
	discoverer := NewGradleStructureDiscoverer()
	if discoverer.Name() != "GradleStructureDiscoverer" {
		t.Errorf("Expected name 'GradleStructureDiscoverer', got '%s'", discoverer.Name())
	}
}