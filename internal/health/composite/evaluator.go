package composite

import (
	"fmt"
	"regexp"
	"strings"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// LogicalOperator represents supported logical operators
type LogicalOperator string

const (
	// AND requires all operands to be true
	AND LogicalOperator = "AND"
	// OR requires at least one operand to be true
	OR LogicalOperator = "OR"
	// NOT negates the operand
	NOT LogicalOperator = "NOT"
)

// Evaluator provides logical expression evaluation for composite health checks
type Evaluator struct {
	logger *zap.Logger
}

// NewEvaluator creates a new logical expression evaluator
func NewEvaluator(logger *zap.Logger) *Evaluator {
	return &Evaluator{
		logger: logger.With(zap.String("component", "health_evaluator")),
	}
}

// LogicalExpression represents a parsed logical expression
type LogicalExpression struct {
	Operator LogicalOperator     `json:"operator"`
	Operands []ExpressionOperand `json:"operands"`
	Raw      string              `json:"raw"`
}

// ExpressionOperand represents an operand in a logical expression
type ExpressionOperand struct {
	Type       OperandType        `json:"type"`
	CheckName  string             `json:"check_name,omitempty"`
	Expression *LogicalExpression `json:"expression,omitempty"`
	Negated    bool               `json:"negated"`
}

// OperandType defines the type of operand
type OperandType string

const (
	// CheckOperand refers to a health check result
	CheckOperand OperandType = "check"
	// ExpressionOperand refers to a nested expression
	ExpressionOperandType OperandType = "expression"
)

// EvaluationResult contains the result of expression evaluation
type EvaluationResult struct {
	Result     bool               `json:"result"`
	Expression string             `json:"expression"`
	Details    *EvaluationDetails `json:"details"`
	Errors     []string           `json:"errors,omitempty"`
}

// EvaluationDetails provides detailed evaluation information
type EvaluationDetails struct {
	CheckResults   map[string]bool   `json:"check_results"`
	SubExpressions map[string]bool   `json:"sub_expressions"`
	EvaluationPath []string          `json:"evaluation_path"`
	TruthTable     []TruthTableEntry `json:"truth_table,omitempty"`
}

// TruthTableEntry represents an entry in the truth table
type TruthTableEntry struct {
	Inputs []bool `json:"inputs"`
	Output bool   `json:"output"`
}

// EvaluateExpression evaluates a logical expression given health check results
func (e *Evaluator) EvaluateExpression(expression string, results map[string]*HealthResult) (*EvaluationResult, error) {
	if expression == "" {
		// Default to simple AND logic for backward compatibility
		return e.evaluateDefaultLogic(results), nil
	}

	// Parse the expression
	parsed, err := e.parseExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression: %w", err)
	}

	// Evaluate the parsed expression
	result, details, err := e.evaluateParsedExpression(parsed, results)
	if err != nil {
		return &EvaluationResult{
			Result:     false,
			Expression: expression,
			Errors:     []string{err.Error()},
		}, nil
	}

	return &EvaluationResult{
		Result:     result,
		Expression: expression,
		Details:    details,
	}, nil
}

// parseExpression parses a logical expression string into a structured format
func (e *Evaluator) parseExpression(expression string) (*LogicalExpression, error) {
	// Normalize the expression
	normalized := e.normalizeExpression(expression)

	// Simple recursive descent parser
	return e.parseLogicalExpression(normalized)
}

// normalizeExpression normalizes the expression for easier parsing
func (e *Evaluator) normalizeExpression(expression string) string {
	// Convert to uppercase and clean whitespace
	normalized := strings.ToUpper(strings.TrimSpace(expression))

	// Replace common variants
	normalized = strings.ReplaceAll(normalized, "&&", " AND ")
	normalized = strings.ReplaceAll(normalized, "||", " OR ")
	normalized = strings.ReplaceAll(normalized, "!", "NOT ")

	// Clean multiple spaces
	normalized = regexp.MustCompile(`\s+`).ReplaceAllString(normalized, " ")

	return strings.TrimSpace(normalized)
}

// parseLogicalExpression parses a logical expression recursively
func (e *Evaluator) parseLogicalExpression(expression string) (*LogicalExpression, error) {
	expression = strings.TrimSpace(expression)

	// Handle parentheses
	if strings.HasPrefix(expression, "(") && strings.HasSuffix(expression, ")") {
		// Remove outer parentheses and parse inner expression
		inner := expression[1 : len(expression)-1]
		return e.parseLogicalExpression(inner)
	}

	// Look for OR operators (lowest precedence)
	if parts := e.splitByOperator(expression, "OR"); len(parts) > 1 {
		return e.createLogicalExpression(OR, parts, expression)
	}

	// Look for AND operators (higher precedence)
	if parts := e.splitByOperator(expression, "AND"); len(parts) > 1 {
		return e.createLogicalExpression(AND, parts, expression)
	}

	// Handle NOT operator
	if strings.HasPrefix(expression, "NOT ") {
		inner := strings.TrimSpace(expression[4:])
		innerExpr, err := e.parseLogicalExpression(inner)
		if err != nil {
			return nil, err
		}

		return &LogicalExpression{
			Operator: NOT,
			Operands: []ExpressionOperand{
				{
					Type:       ExpressionOperandType,
					Expression: innerExpr,
					Negated:    false,
				},
			},
			Raw: expression,
		}, nil
	}

	// Single check name
	if e.isValidCheckName(expression) {
		return &LogicalExpression{
			Operator: AND, // Single operand AND
			Operands: []ExpressionOperand{
				{
					Type:      CheckOperand,
					CheckName: expression,
					Negated:   false,
				},
			},
			Raw: expression,
		}, nil
	}

	return nil, fmt.Errorf("invalid expression: %s", expression)
}

// splitByOperator splits expression by the given operator, respecting parentheses
func (e *Evaluator) splitByOperator(expression string, operator string) []string {
	parts := []string{}
	current := ""
	parenLevel := 0
	words := strings.Fields(expression)

	i := 0
	for i < len(words) {
		word := words[i]

		// Count parentheses
		parenLevel += strings.Count(word, "(") - strings.Count(word, ")")

		if word == operator && parenLevel == 0 {
			// Found operator at top level
			if current != "" {
				parts = append(parts, strings.TrimSpace(current))
				current = ""
			}
		} else {
			if current != "" {
				current += " "
			}
			current += word
		}

		i++
	}

	if current != "" {
		parts = append(parts, strings.TrimSpace(current))
	}

	return parts
}

// createLogicalExpression creates a logical expression from operator and parts
func (e *Evaluator) createLogicalExpression(operator LogicalOperator, parts []string, raw string) (*LogicalExpression, error) {
	operands := make([]ExpressionOperand, 0, len(parts))

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}

		if e.isValidCheckName(part) {
			operands = append(operands, ExpressionOperand{
				Type:      CheckOperand,
				CheckName: part,
				Negated:   false,
			})
		} else {
			// Parse as sub-expression
			subExpr, err := e.parseLogicalExpression(part)
			if err != nil {
				return nil, fmt.Errorf("failed to parse sub-expression '%s': %w", part, err)
			}

			operands = append(operands, ExpressionOperand{
				Type:       ExpressionOperandType,
				Expression: subExpr,
				Negated:    false,
			})
		}
	}

	return &LogicalExpression{
		Operator: operator,
		Operands: operands,
		Raw:      raw,
	}, nil
}

// isValidCheckName validates if a string is a valid health check name
func (e *Evaluator) isValidCheckName(name string) bool {
	// Simple validation - alphanumeric, hyphens, underscores
	matched, _ := regexp.MatchString(`^[a-zA-Z0-9_-]+$`, name)
	return matched && name != "AND" && name != "OR" && name != "NOT"
}

// evaluateParsedExpression evaluates a parsed logical expression
func (e *Evaluator) evaluateParsedExpression(expr *LogicalExpression, results map[string]*HealthResult) (bool, *EvaluationDetails, error) {
	details := &EvaluationDetails{
		CheckResults:   make(map[string]bool),
		SubExpressions: make(map[string]bool),
		EvaluationPath: []string{},
	}

	result, err := e.evaluateExpressionRecursive(expr, results, details)
	return result, details, err
}

// evaluateExpressionRecursive recursively evaluates logical expressions
func (e *Evaluator) evaluateExpressionRecursive(expr *LogicalExpression, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	details.EvaluationPath = append(details.EvaluationPath, expr.Raw)

	switch expr.Operator {
	case AND:
		return e.evaluateAND(expr.Operands, results, details)
	case OR:
		return e.evaluateOR(expr.Operands, results, details)
	case NOT:
		return e.evaluateNOT(expr.Operands, results, details)
	default:
		return false, fmt.Errorf("unknown operator: %s", expr.Operator)
	}
}

// evaluateAND evaluates AND logic
func (e *Evaluator) evaluateAND(operands []ExpressionOperand, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	for _, operand := range operands {
		result, err := e.evaluateOperand(operand, results, details)
		if err != nil {
			return false, err
		}

		if !result {
			return false, nil // Short-circuit on first false
		}
	}
	return true, nil
}

// evaluateOR evaluates OR logic
func (e *Evaluator) evaluateOR(operands []ExpressionOperand, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	for _, operand := range operands {
		result, err := e.evaluateOperand(operand, results, details)
		if err != nil {
			return false, err
		}

		if result {
			return true, nil // Short-circuit on first true
		}
	}
	return false, nil
}

// evaluateNOT evaluates NOT logic
func (e *Evaluator) evaluateNOT(operands []ExpressionOperand, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	if len(operands) != 1 {
		return false, fmt.Errorf("NOT operator requires exactly one operand, got %d", len(operands))
	}

	result, err := e.evaluateOperand(operands[0], results, details)
	if err != nil {
		return false, err
	}

	return !result, nil
}

// evaluateOperand evaluates a single operand
func (e *Evaluator) evaluateOperand(operand ExpressionOperand, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	var result bool
	var err error

	switch operand.Type {
	case CheckOperand:
		result, err = e.evaluateCheckOperand(operand.CheckName, results, details)
	case ExpressionOperandType:
		result, err = e.evaluateExpressionRecursive(operand.Expression, results, details)
	default:
		return false, fmt.Errorf("unknown operand type: %s", operand.Type)
	}

	if err != nil {
		return false, err
	}

	if operand.Negated {
		result = !result
	}

	return result, nil
}

// evaluateCheckOperand evaluates a health check operand
func (e *Evaluator) evaluateCheckOperand(checkName string, results map[string]*HealthResult, details *EvaluationDetails) (bool, error) {
	healthResult, exists := results[checkName]
	if !exists {
		return false, fmt.Errorf("health check '%s' not found in results", checkName)
	}

	result := healthResult.Healthy
	details.CheckResults[checkName] = result

	e.logger.Debug("Evaluated health check operand",
		zap.String("check_name", checkName),
		zap.Bool("result", result))

	return result, nil
}

// evaluateDefaultLogic provides simple AND logic for backward compatibility
func (e *Evaluator) evaluateDefaultLogic(results map[string]*HealthResult) *EvaluationResult {
	checkResults := make(map[string]bool)
	overallResult := true

	for checkName, healthResult := range results {
		healthy := healthResult.Healthy
		checkResults[checkName] = healthy

		if !healthy {
			overallResult = false
		}
	}

	return &EvaluationResult{
		Result:     overallResult,
		Expression: "DEFAULT_AND",
		Details: &EvaluationDetails{
			CheckResults:   checkResults,
			SubExpressions: make(map[string]bool),
			EvaluationPath: []string{"DEFAULT_AND"},
		},
	}
}

// ValidateExpression validates a logical expression without evaluating it
func (e *Evaluator) ValidateExpression(expression string, healthChecks []roostv1alpha1.HealthCheckSpec) []string {
	var warnings []string

	if expression == "" {
		return warnings
	}

	// Build set of valid check names
	checkNames := make(map[string]bool)
	for _, check := range healthChecks {
		checkNames[check.Name] = true
	}

	// Parse expression
	parsed, err := e.parseExpression(expression)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Invalid expression syntax: %v", err))
		return warnings
	}

	// Validate check names in expression
	checkNamesInExpr := e.extractCheckNames(parsed)
	for _, checkName := range checkNamesInExpr {
		if !checkNames[checkName] {
			warnings = append(warnings,
				fmt.Sprintf("Expression references non-existent health check: %s", checkName))
		}
	}

	return warnings
}

// extractCheckNames extracts all check names referenced in an expression
func (e *Evaluator) extractCheckNames(expr *LogicalExpression) []string {
	var checkNames []string

	for _, operand := range expr.Operands {
		switch operand.Type {
		case CheckOperand:
			checkNames = append(checkNames, operand.CheckName)
		case ExpressionOperandType:
			subNames := e.extractCheckNames(operand.Expression)
			checkNames = append(checkNames, subNames...)
		}
	}

	return checkNames
}

// GenerateTruthTable generates a truth table for the expression
func (e *Evaluator) GenerateTruthTable(expression string, healthChecks []roostv1alpha1.HealthCheckSpec) (*TruthTable, error) {
	if expression == "" {
		return nil, fmt.Errorf("empty expression")
	}

	parsed, err := e.parseExpression(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expression: %w", err)
	}

	checkNames := e.extractCheckNames(parsed)
	if len(checkNames) == 0 {
		return nil, fmt.Errorf("no health checks found in expression")
	}

	// Generate all possible combinations
	numChecks := len(checkNames)
	numCombinations := 1 << numChecks // 2^n combinations

	truthTable := &TruthTable{
		Expression: expression,
		CheckNames: checkNames,
		Entries:    make([]TruthTableEntry, 0, numCombinations),
	}

	for i := 0; i < numCombinations; i++ {
		// Create test results for this combination
		testResults := make(map[string]*HealthResult)
		inputs := make([]bool, numChecks)

		for j, checkName := range checkNames {
			healthy := (i>>j)&1 == 1
			inputs[j] = healthy
			testResults[checkName] = &HealthResult{
				Healthy: healthy,
				Message: fmt.Sprintf("Test case %d", i),
			}
		}

		// Evaluate expression with these test results
		evalResult, err := e.EvaluateExpression(expression, testResults)
		if err != nil {
			return nil, fmt.Errorf("failed to evaluate test case %d: %w", i, err)
		}

		truthTable.Entries = append(truthTable.Entries, TruthTableEntry{
			Inputs: inputs,
			Output: evalResult.Result,
		})
	}

	return truthTable, nil
}

// TruthTable represents a complete truth table for an expression
type TruthTable struct {
	Expression string            `json:"expression"`
	CheckNames []string          `json:"check_names"`
	Entries    []TruthTableEntry `json:"entries"`
}

// OptimizeExpression attempts to optimize a logical expression
func (e *Evaluator) OptimizeExpression(expression string) (string, error) {
	// For now, return the original expression
	// Future optimizations could include:
	// - Removing redundant terms
	// - Applying De Morgan's laws
	// - Simplifying nested expressions
	return expression, nil
}
