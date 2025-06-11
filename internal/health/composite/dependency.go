package composite

import (
	"fmt"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// DependencyResolver manages health check dependencies and execution order
type DependencyResolver struct {
	logger *zap.Logger
}

// NewDependencyResolver creates a new dependency resolver
func NewDependencyResolver(logger *zap.Logger) *DependencyResolver {
	return &DependencyResolver{
		logger: logger.With(zap.String("component", "dependency_resolver")),
	}
}

// DependencyGraph represents the dependency relationships between health checks
type DependencyGraph struct {
	nodes        map[string]*DependencyNode
	edges        map[string][]string // checkName -> list of dependencies
	reverseEdges map[string][]string // checkName -> list of dependents
}

// DependencyNode represents a health check in the dependency graph
type DependencyNode struct {
	Name         string   `json:"name"`
	Dependencies []string `json:"dependencies"`
	Dependents   []string `json:"dependents"`
	InDegree     int      `json:"in_degree"`
}

// ResolveExecutionOrder determines the execution order for health checks based on dependencies
func (dr *DependencyResolver) ResolveExecutionOrder(healthChecks []roostv1alpha1.HealthCheckSpec) ([]string, error) {
	if len(healthChecks) == 0 {
		return []string{}, nil
	}

	// Build dependency graph
	graph, err := dr.buildDependencyGraph(healthChecks)
	if err != nil {
		return nil, fmt.Errorf("failed to build dependency graph: %w", err)
	}

	// Detect cycles
	if err := dr.detectCycles(graph); err != nil {
		return nil, fmt.Errorf("circular dependency detected: %w", err)
	}

	// Perform topological sort
	order, err := dr.topologicalSort(graph)
	if err != nil {
		return nil, fmt.Errorf("failed to perform topological sort: %w", err)
	}

	dr.logger.Debug("Resolved health check execution order",
		zap.Strings("order", order),
		zap.Int("total_checks", len(order)))

	return order, nil
}

// buildDependencyGraph constructs a dependency graph from health check specifications
func (dr *DependencyResolver) buildDependencyGraph(healthChecks []roostv1alpha1.HealthCheckSpec) (*DependencyGraph, error) {
	graph := &DependencyGraph{
		nodes:        make(map[string]*DependencyNode),
		edges:        make(map[string][]string),
		reverseEdges: make(map[string][]string),
	}

	// First pass: create all nodes
	for _, check := range healthChecks {
		graph.nodes[check.Name] = &DependencyNode{
			Name:         check.Name,
			Dependencies: []string{},
			Dependents:   []string{},
			InDegree:     0,
		}
		graph.edges[check.Name] = []string{}
		graph.reverseEdges[check.Name] = []string{}
	}

	// Second pass: add dependencies
	for _, check := range healthChecks {
		dependencies := dr.extractDependencies(check)

		for _, dep := range dependencies {
			// Validate that dependency exists
			if _, exists := graph.nodes[dep]; !exists {
				return nil, fmt.Errorf("health check '%s' depends on non-existent check '%s'", check.Name, dep)
			}

			// Add edge from dependency to dependent
			graph.edges[dep] = append(graph.edges[dep], check.Name)
			graph.reverseEdges[check.Name] = append(graph.reverseEdges[check.Name], dep)

			// Update node information
			graph.nodes[check.Name].Dependencies = append(graph.nodes[check.Name].Dependencies, dep)
			graph.nodes[dep].Dependents = append(graph.nodes[dep].Dependents, check.Name)
			graph.nodes[check.Name].InDegree++
		}
	}

	return graph, nil
}

// extractDependencies extracts dependency information from a health check specification
func (dr *DependencyResolver) extractDependencies(check roostv1alpha1.HealthCheckSpec) []string {
	var dependencies []string

	// TODO: Add DependsOn field to HealthCheckSpec in CRD
	// For now, we'll extract dependencies from a hypothetical field
	// In the future, this would look like: dependencies = check.DependsOn

	// Placeholder implementation - in real implementation this would extract from CRD
	// For demonstration, we'll return empty dependencies
	return dependencies
}

// detectCycles detects circular dependencies in the graph using DFS
func (dr *DependencyResolver) detectCycles(graph *DependencyGraph) error {
	visited := make(map[string]bool)
	recursionStack := make(map[string]bool)

	for nodeName := range graph.nodes {
		if !visited[nodeName] {
			if dr.hasCycleDFS(graph, nodeName, visited, recursionStack) {
				return fmt.Errorf("circular dependency detected involving node '%s'", nodeName)
			}
		}
	}

	return nil
}

// hasCycleDFS performs DFS to detect cycles
func (dr *DependencyResolver) hasCycleDFS(graph *DependencyGraph, node string, visited, recursionStack map[string]bool) bool {
	visited[node] = true
	recursionStack[node] = true

	// Check all dependents (children in the graph)
	for _, dependent := range graph.edges[node] {
		if !visited[dependent] {
			if dr.hasCycleDFS(graph, dependent, visited, recursionStack) {
				return true
			}
		} else if recursionStack[dependent] {
			return true
		}
	}

	recursionStack[node] = false
	return false
}

// topologicalSort performs Kahn's algorithm for topological sorting
func (dr *DependencyResolver) topologicalSort(graph *DependencyGraph) ([]string, error) {
	// Initialize in-degree counts
	inDegree := make(map[string]int)
	for nodeName, node := range graph.nodes {
		inDegree[nodeName] = node.InDegree
	}

	// Find all nodes with no incoming edges
	queue := []string{}
	for nodeName, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, nodeName)
		}
	}

	result := []string{}

	// Process nodes in topological order
	for len(queue) > 0 {
		// Remove a node from the queue
		current := queue[0]
		queue = queue[1:]
		result = append(result, current)

		// For each dependent of the current node
		for _, dependent := range graph.edges[current] {
			// Remove the edge
			inDegree[dependent]--

			// If dependent has no more incoming edges, add to queue
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
	}

	// Check if all nodes were processed (no cycles)
	if len(result) != len(graph.nodes) {
		return nil, fmt.Errorf("graph contains cycles, cannot perform topological sort")
	}

	return result, nil
}

// ValidateDependencies validates that all dependencies are valid
func (dr *DependencyResolver) ValidateDependencies(healthChecks []roostv1alpha1.HealthCheckSpec) []string {
	var warnings []string
	checkNames := make(map[string]bool)

	// Build set of valid check names
	for _, check := range healthChecks {
		checkNames[check.Name] = true
	}

	// Validate dependencies
	for _, check := range healthChecks {
		dependencies := dr.extractDependencies(check)

		for _, dep := range dependencies {
			if !checkNames[dep] {
				warnings = append(warnings,
					fmt.Sprintf("Health check '%s' depends on non-existent check '%s'", check.Name, dep))
			}

			if dep == check.Name {
				warnings = append(warnings,
					fmt.Sprintf("Health check '%s' cannot depend on itself", check.Name))
			}
		}
	}

	return warnings
}

// GetDependencyInfo returns detailed dependency information
func (dr *DependencyResolver) GetDependencyInfo(healthChecks []roostv1alpha1.HealthCheckSpec) (*DependencyInfo, error) {
	graph, err := dr.buildDependencyGraph(healthChecks)
	if err != nil {
		return nil, err
	}

	order, err := dr.topologicalSort(graph)
	if err != nil {
		return nil, err
	}

	// Calculate dependency levels
	levels := dr.calculateDependencyLevels(graph, order)

	// Find critical path
	criticalPath := dr.findCriticalPath(graph)

	return &DependencyInfo{
		ExecutionOrder:   order,
		DependencyLevels: levels,
		CriticalPath:     criticalPath,
		TotalChecks:      len(healthChecks),
		DependencyCount:  dr.countTotalDependencies(graph),
		MaxDepth:         dr.calculateMaxDepth(graph),
		Nodes:            graph.nodes,
	}, nil
}

// DependencyInfo provides comprehensive dependency analysis
type DependencyInfo struct {
	ExecutionOrder   []string                   `json:"execution_order"`
	DependencyLevels map[string]int             `json:"dependency_levels"`
	CriticalPath     []string                   `json:"critical_path"`
	TotalChecks      int                        `json:"total_checks"`
	DependencyCount  int                        `json:"dependency_count"`
	MaxDepth         int                        `json:"max_depth"`
	Nodes            map[string]*DependencyNode `json:"nodes"`
}

// calculateDependencyLevels assigns level numbers based on dependency depth
func (dr *DependencyResolver) calculateDependencyLevels(graph *DependencyGraph, order []string) map[string]int {
	levels := make(map[string]int)

	// Process nodes in topological order
	for _, nodeName := range order {
		maxDepLevel := 0

		// Find the maximum level of all dependencies
		for _, dep := range graph.nodes[nodeName].Dependencies {
			if depLevel, exists := levels[dep]; exists {
				if depLevel >= maxDepLevel {
					maxDepLevel = depLevel + 1
				}
			}
		}

		levels[nodeName] = maxDepLevel
	}

	return levels
}

// findCriticalPath identifies the longest dependency chain
func (dr *DependencyResolver) findCriticalPath(graph *DependencyGraph) []string {
	visited := make(map[string]bool)
	var longestPath []string
	maxLength := 0

	// Try starting from each node to find the longest path
	for nodeName := range graph.nodes {
		if !visited[nodeName] {
			path := dr.findLongestPath(graph, nodeName, visited)
			if len(path) > maxLength {
				maxLength = len(path)
				longestPath = path
			}
		}
	}

	return longestPath
}

// findLongestPath finds the longest path starting from a given node
func (dr *DependencyResolver) findLongestPath(graph *DependencyGraph, startNode string, globalVisited map[string]bool) []string {
	localVisited := make(map[string]bool)
	return dr.dfsLongestPath(graph, startNode, localVisited, globalVisited)
}

// dfsLongestPath performs DFS to find the longest path
func (dr *DependencyResolver) dfsLongestPath(graph *DependencyGraph, node string, localVisited, globalVisited map[string]bool) []string {
	localVisited[node] = true
	globalVisited[node] = true

	var longestSubPath []string
	maxLength := 0

	// Explore all dependents
	for _, dependent := range graph.edges[node] {
		if !localVisited[dependent] {
			subPath := dr.dfsLongestPath(graph, dependent, localVisited, globalVisited)
			if len(subPath) > maxLength {
				maxLength = len(subPath)
				longestSubPath = subPath
			}
		}
	}

	// Current path is this node plus the longest sub-path
	result := []string{node}
	result = append(result, longestSubPath...)

	localVisited[node] = false
	return result
}

// countTotalDependencies counts the total number of dependency relationships
func (dr *DependencyResolver) countTotalDependencies(graph *DependencyGraph) int {
	count := 0
	for _, node := range graph.nodes {
		count += len(node.Dependencies)
	}
	return count
}

// calculateMaxDepth calculates the maximum dependency depth
func (dr *DependencyResolver) calculateMaxDepth(graph *DependencyGraph) int {
	visited := make(map[string]bool)
	maxDepth := 0

	for nodeName := range graph.nodes {
		if !visited[nodeName] {
			depth := dr.calculateNodeDepth(graph, nodeName, visited, make(map[string]bool))
			if depth > maxDepth {
				maxDepth = depth
			}
		}
	}

	return maxDepth
}

// calculateNodeDepth calculates the maximum depth from a given node
func (dr *DependencyResolver) calculateNodeDepth(graph *DependencyGraph, node string, globalVisited, localVisited map[string]bool) int {
	if localVisited[node] {
		return 0 // Avoid infinite loops
	}

	localVisited[node] = true
	globalVisited[node] = true

	maxChildDepth := 0
	for _, dependent := range graph.edges[node] {
		if !localVisited[dependent] {
			childDepth := dr.calculateNodeDepth(graph, dependent, globalVisited, localVisited)
			if childDepth > maxChildDepth {
				maxChildDepth = childDepth
			}
		}
	}

	localVisited[node] = false
	return maxChildDepth + 1
}

// OptimizeExecutionOrder attempts to optimize execution order for parallel execution
func (dr *DependencyResolver) OptimizeExecutionOrder(healthChecks []roostv1alpha1.HealthCheckSpec) (*OptimizedExecution, error) {
	graph, err := dr.buildDependencyGraph(healthChecks)
	if err != nil {
		return nil, err
	}

	// Group checks by dependency level for potential parallel execution
	order, err := dr.topologicalSort(graph)
	if err != nil {
		return nil, err
	}

	levels := dr.calculateDependencyLevels(graph, order)
	levelGroups := make(map[int][]string)

	for checkName, level := range levels {
		levelGroups[level] = append(levelGroups[level], checkName)
	}

	// Create execution phases
	maxLevel := 0
	for level := range levelGroups {
		if level > maxLevel {
			maxLevel = level
		}
	}

	phases := make([]ExecutionPhase, maxLevel+1)
	for level := 0; level <= maxLevel; level++ {
		phases[level] = ExecutionPhase{
			Level:  level,
			Checks: levelGroups[level],
		}
	}

	return &OptimizedExecution{
		SequentialOrder: order,
		ParallelPhases:  phases,
		MaxParallelism:  dr.calculateMaxParallelism(levelGroups),
		TotalPhases:     len(phases),
	}, nil
}

// OptimizedExecution provides both sequential and parallel execution strategies
type OptimizedExecution struct {
	SequentialOrder []string         `json:"sequential_order"`
	ParallelPhases  []ExecutionPhase `json:"parallel_phases"`
	MaxParallelism  int              `json:"max_parallelism"`
	TotalPhases     int              `json:"total_phases"`
}

// ExecutionPhase represents a phase of parallel execution
type ExecutionPhase struct {
	Level  int      `json:"level"`
	Checks []string `json:"checks"`
}

// calculateMaxParallelism finds the maximum number of checks that can run in parallel
func (dr *DependencyResolver) calculateMaxParallelism(levelGroups map[int][]string) int {
	maxParallel := 0
	for _, checks := range levelGroups {
		if len(checks) > maxParallel {
			maxParallel = len(checks)
		}
	}
	return maxParallel
}
