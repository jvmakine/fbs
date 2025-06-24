package graph

import (
	"crypto/sha256"
	"fmt"
	"sort"
)

// ComputeTaskHash computes a hash for a task including its dependencies
func ComputeTaskHash(task Task) string {
	h := sha256.New()
	
	// Add the task's own hash
	h.Write([]byte(task.Hash()))
	
	// Add dependency hashes (sorted for consistency)
	var depHashes []string
	for _, dep := range task.Dependencies() {
		depHashes = append(depHashes, ComputeTaskHash(dep))
	}
	sort.Strings(depHashes)
	
	for _, depHash := range depHashes {
		h.Write([]byte(depHash))
	}
	
	return fmt.Sprintf("%x", h.Sum(nil))
}