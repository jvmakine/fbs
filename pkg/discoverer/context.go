package discoverer

import (
	"context"
	"reflect"
)

// BuildContext stores build metadata that flows down the directory tree
type BuildContext struct {
	// metadata stores context objects by their type
	metadata map[reflect.Type]interface{}
}

// NewBuildContext creates a new empty BuildContext
func NewBuildContext() *BuildContext {
	return &BuildContext{
		metadata: make(map[reflect.Type]interface{}),
	}
}

// Copy creates a deep copy of the BuildContext for passing to subdirectories
func (bc *BuildContext) Copy() *BuildContext {
	newContext := &BuildContext{
		metadata: make(map[reflect.Type]interface{}),
	}
	
	// Copy all metadata entries
	for k, v := range bc.metadata {
		newContext.metadata[k] = v
	}
	
	return newContext
}

// Set adds or updates a context object by its type
func (bc *BuildContext) Set(obj interface{}) {
	if obj == nil {
		return
	}
	objType := reflect.TypeOf(obj)
	bc.metadata[objType] = obj
}

// Get retrieves a context object by type. Returns nil if not found.
func (bc *BuildContext) Get(objType reflect.Type) interface{} {
	return bc.metadata[objType]
}

// GetByExample retrieves a context object using an example of the desired type.
// This is a convenience method for type-safe retrieval.
// 
// Example usage:
//   versions := ctx.GetByExample((*GradleArtefactVersions)(nil)).(*GradleArtefactVersions)
//   if versions != nil { ... }
func (bc *BuildContext) GetByExample(example interface{}) interface{} {
	if example == nil {
		return nil
	}
	objType := reflect.TypeOf(example)
	
	// Handle pointer types - get the element type
	if objType.Kind() == reflect.Ptr {
		objType = objType.Elem()
	}
	
	// Look for both pointer and value types
	if obj := bc.metadata[objType]; obj != nil {
		return obj
	}
	if obj := bc.metadata[reflect.PtrTo(objType)]; obj != nil {
		return obj
	}
	
	return nil
}

// Has checks if a context object of the given type exists
func (bc *BuildContext) Has(objType reflect.Type) bool {
	_, exists := bc.metadata[objType]
	return exists
}

// ContextDiscoverer discovers and populates BuildContext metadata for a directory
type ContextDiscoverer interface {
	// Name returns the name of this context discoverer
	Name() string
	
	// DiscoverContext examines a directory and adds metadata to the BuildContext
	// The context is modified in-place and will be passed down to subdirectories
	DiscoverContext(ctx context.Context, path string, buildContext *BuildContext) error
}