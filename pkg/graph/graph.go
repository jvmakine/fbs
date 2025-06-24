package graph

import (
	"fmt"
)

// Graph represents a directed acyclic graph of tasks
type Graph struct {
	tasks []Task
	edges map[string][]string // task ID -> list of dependency task IDs
}

// NewGraph creates a new empty graph
func NewGraph() *Graph {
	return &Graph{
		tasks: make([]Task, 0),
		edges: make(map[string][]string),
	}
}

// AddTask adds a task to the graph
func (g *Graph) AddTask(task Task) error {
	// Check if task already exists
	for _, existing := range g.tasks {
		if existing.ID() == task.ID() {
			return fmt.Errorf("task with ID %s already exists", task.ID())
		}
	}
	
	g.tasks = append(g.tasks, task)
	
	// Add edges for dependencies
	var depIDs []string
	for _, dep := range task.Dependencies() {
		depIDs = append(depIDs, dep.ID())
	}
	g.edges[task.ID()] = depIDs
	
	return nil
}

// GetTask returns a task by its ID
func (g *Graph) GetTask(id string) (Task, error) {
	for _, task := range g.tasks {
		if task.ID() == id {
			return task, nil
		}
	}
	return nil, fmt.Errorf("task with ID %s not found", id)
}

// GetTasks returns all tasks in the graph
func (g *Graph) GetTasks() []Task {
	return g.tasks
}

// TopologicalSort returns tasks in topological order (dependencies first)
func (g *Graph) TopologicalSort() ([]Task, error) {
	// Kahn's algorithm for topological sorting
	inDegree := make(map[string]int)
	
	// Initialize in-degree count (number of dependencies for each task)
	for _, task := range g.tasks {
		inDegree[task.ID()] = len(g.edges[task.ID()])
	}
	
	// Find tasks with no dependencies (in-degree = 0)
	var queue []string
	for taskID, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, taskID)
		}
	}
	
	var result []Task
	
	for len(queue) > 0 {
		// Pop from queue
		current := queue[0]
		queue = queue[1:]
		
		// Add to result
		task, err := g.GetTask(current)
		if err != nil {
			return nil, err
		}
		result = append(result, task)
		
		// For each task that depends on the current task, reduce its in-degree
		for _, otherTask := range g.tasks {
			otherTaskDeps := g.edges[otherTask.ID()]
			for _, dep := range otherTaskDeps {
				if dep == current {
					inDegree[otherTask.ID()]--
					if inDegree[otherTask.ID()] == 0 {
						queue = append(queue, otherTask.ID())
					}
				}
			}
		}
	}
	
	// Check for cycles
	if len(result) != len(g.tasks) {
		return nil, fmt.Errorf("cycle detected in task graph")
	}
	
	return result, nil
}