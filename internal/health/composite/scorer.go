package composite

import (
	"math"

	"go.uber.org/zap"
)

// ScoringStrategy defines different scoring strategies
type ScoringStrategy string

const (
	// WeightedAverage uses weighted average scoring
	WeightedAverage ScoringStrategy = "weighted_average"
	// WeightedSum uses weighted sum scoring (normalized)
	WeightedSum ScoringStrategy = "weighted_sum"
	// Threshold uses threshold-based scoring
	Threshold ScoringStrategy = "threshold"
)

// ScoringConfig defines configuration for health scoring
type ScoringConfig struct {
	Strategy          ScoringStrategy `json:"strategy"`
	MinimumScore      float64         `json:"minimum_score"`
	HealthyThreshold  float64         `json:"healthy_threshold"`
	DegradedThreshold float64         `json:"degraded_threshold"`
	PenaltyMultiplier float64         `json:"penalty_multiplier"`
	EnablePenalties   bool            `json:"enable_penalties"`
	FailureWeight     float64         `json:"failure_weight"`
}

// DefaultScoringConfig returns the default scoring configuration
func DefaultScoringConfig() ScoringConfig {
	return ScoringConfig{
		Strategy:          WeightedAverage,
		MinimumScore:      0.0,
		HealthyThreshold:  0.8,
		DegradedThreshold: 0.5,
		PenaltyMultiplier: 1.5,
		EnablePenalties:   true,
		FailureWeight:     2.0,
	}
}

// Scorer provides weighted scoring algorithms for composite health evaluation
type Scorer struct {
	config ScoringConfig
	logger *zap.Logger
}

// NewScorer creates a new scorer with default configuration
func NewScorer(logger *zap.Logger) *Scorer {
	return &Scorer{
		config: DefaultScoringConfig(),
		logger: logger.With(zap.String("component", "health_scorer")),
	}
}

// NewScorerWithConfig creates a new scorer with custom configuration
func NewScorerWithConfig(logger *zap.Logger, config ScoringConfig) *Scorer {
	return &Scorer{
		config: config,
		logger: logger.With(zap.String("component", "health_scorer")),
	}
}

// CalculateWeightedScore calculates the weighted health score using the configured strategy
func (s *Scorer) CalculateWeightedScore(results map[string]*HealthResult, weights map[string]float64) float64 {
	if len(results) == 0 {
		return 0.0
	}

	switch s.config.Strategy {
	case WeightedAverage:
		return s.calculateWeightedAverage(results, weights)
	case WeightedSum:
		return s.calculateWeightedSum(results, weights)
	case Threshold:
		return s.calculateThresholdScore(results, weights)
	default:
		s.logger.Warn("Unknown scoring strategy, using weighted average",
			zap.String("strategy", string(s.config.Strategy)))
		return s.calculateWeightedAverage(results, weights)
	}
}

// calculateWeightedAverage calculates weighted average health score
func (s *Scorer) calculateWeightedAverage(results map[string]*HealthResult, weights map[string]float64) float64 {
	totalWeight := 0.0
	weightedScore := 0.0

	for checkName, result := range results {
		weight := s.getWeight(checkName, weights)
		totalWeight += weight

		if result.Healthy {
			weightedScore += weight
		} else if s.config.EnablePenalties {
			// Apply penalty for failures
			penalty := weight * s.config.PenaltyMultiplier
			weightedScore -= penalty
		}
	}

	if totalWeight == 0 {
		return 0.0
	}

	score := weightedScore / totalWeight

	// Ensure score is within bounds
	score = math.Max(score, s.config.MinimumScore)
	score = math.Min(score, 1.0)

	return score
}

// calculateWeightedSum calculates normalized weighted sum health score
func (s *Scorer) calculateWeightedSum(results map[string]*HealthResult, weights map[string]float64) float64 {
	totalPossibleScore := 0.0
	actualScore := 0.0

	for checkName, result := range results {
		weight := s.getWeight(checkName, weights)
		totalPossibleScore += weight

		if result.Healthy {
			actualScore += weight
		} else {
			// Apply failure weight for unhealthy checks
			failureDeduction := weight * s.config.FailureWeight
			actualScore -= failureDeduction
		}
	}

	if totalPossibleScore == 0 {
		return 0.0
	}

	score := actualScore / totalPossibleScore

	// Normalize to 0-1 range
	score = math.Max(score, s.config.MinimumScore)
	score = math.Min(score, 1.0)

	return score
}

// calculateThresholdScore calculates score based on threshold achievement
func (s *Scorer) calculateThresholdScore(results map[string]*HealthResult, weights map[string]float64) float64 {
	totalChecks := len(results)
	passedChecks := 0
	totalWeight := 0.0
	passedWeight := 0.0

	for checkName, result := range results {
		weight := s.getWeight(checkName, weights)
		totalWeight += weight

		if result.Healthy {
			passedChecks++
			passedWeight += weight
		}
	}

	// Calculate both count-based and weight-based ratios
	countRatio := float64(passedChecks) / float64(totalChecks)
	weightRatio := 0.0
	if totalWeight > 0 {
		weightRatio = passedWeight / totalWeight
	}

	// Use the more stringent of the two ratios
	score := math.Min(countRatio, weightRatio)

	// Apply threshold logic
	if score >= s.config.HealthyThreshold {
		return 1.0
	} else if score >= s.config.DegradedThreshold {
		// Linear interpolation between degraded and healthy thresholds
		range_ := s.config.HealthyThreshold - s.config.DegradedThreshold
		position := score - s.config.DegradedThreshold
		return s.config.DegradedThreshold + (position/range_)*(1.0-s.config.DegradedThreshold)
	} else {
		// Linear interpolation between minimum and degraded thresholds
		range_ := s.config.DegradedThreshold - s.config.MinimumScore
		if range_ <= 0 {
			return s.config.MinimumScore
		}
		position := score - s.config.MinimumScore
		return s.config.MinimumScore + (position/range_)*s.config.DegradedThreshold
	}
}

// getWeight retrieves the weight for a health check, with fallback to default
func (s *Scorer) getWeight(checkName string, weights map[string]float64) float64 {
	if weight, exists := weights[checkName]; exists && weight > 0 {
		return weight
	}
	return 1.0 // Default weight
}

// GetHealthStatus determines health status based on score
func (s *Scorer) GetHealthStatus(score float64) string {
	if score >= s.config.HealthyThreshold {
		return "healthy"
	} else if score >= s.config.DegradedThreshold {
		return "degraded"
	} else {
		return "unhealthy"
	}
}

// CalculateDetailedScore provides detailed scoring information
func (s *Scorer) CalculateDetailedScore(results map[string]*HealthResult, weights map[string]float64) *DetailedScore {
	totalWeight := 0.0
	healthyWeight := 0.0
	unhealthyWeight := 0.0
	healthyChecks := 0
	unhealthyChecks := 0

	checkScores := make(map[string]CheckScore)

	for checkName, result := range results {
		weight := s.getWeight(checkName, weights)
		totalWeight += weight

		checkScore := CheckScore{
			Name:    checkName,
			Healthy: result.Healthy,
			Weight:  weight,
		}

		if result.Healthy {
			healthyChecks++
			healthyWeight += weight
			checkScore.Score = weight
		} else {
			unhealthyChecks++
			unhealthyWeight += weight
			if s.config.EnablePenalties {
				checkScore.Score = -weight * s.config.PenaltyMultiplier
			} else {
				checkScore.Score = 0.0
			}
		}

		checkScores[checkName] = checkScore
	}

	overallScore := s.CalculateWeightedScore(results, weights)
	healthStatus := s.GetHealthStatus(overallScore)

	return &DetailedScore{
		OverallScore:    overallScore,
		HealthStatus:    healthStatus,
		TotalWeight:     totalWeight,
		HealthyWeight:   healthyWeight,
		UnhealthyWeight: unhealthyWeight,
		HealthyChecks:   healthyChecks,
		UnhealthyChecks: unhealthyChecks,
		CheckScores:     checkScores,
		Strategy:        string(s.config.Strategy),
		Config:          s.config,
	}
}

// DetailedScore provides comprehensive scoring information
type DetailedScore struct {
	OverallScore    float64               `json:"overall_score"`
	HealthStatus    string                `json:"health_status"`
	TotalWeight     float64               `json:"total_weight"`
	HealthyWeight   float64               `json:"healthy_weight"`
	UnhealthyWeight float64               `json:"unhealthy_weight"`
	HealthyChecks   int                   `json:"healthy_checks"`
	UnhealthyChecks int                   `json:"unhealthy_checks"`
	CheckScores     map[string]CheckScore `json:"check_scores"`
	Strategy        string                `json:"strategy"`
	Config          ScoringConfig         `json:"config"`
}

// CheckScore provides scoring information for an individual health check
type CheckScore struct {
	Name    string  `json:"name"`
	Healthy bool    `json:"healthy"`
	Weight  float64 `json:"weight"`
	Score   float64 `json:"score"`
}

// UpdateConfig updates the scorer configuration
func (s *Scorer) UpdateConfig(config ScoringConfig) {
	s.config = config
	s.logger.Info("Updated scorer configuration",
		zap.String("strategy", string(config.Strategy)),
		zap.Float64("healthy_threshold", config.HealthyThreshold),
		zap.Float64("degraded_threshold", config.DegradedThreshold))
}

// GetConfig returns the current scorer configuration
func (s *Scorer) GetConfig() ScoringConfig {
	return s.config
}

// ValidateWeights validates that weights are reasonable
func (s *Scorer) ValidateWeights(weights map[string]float64) []string {
	var warnings []string

	for checkName, weight := range weights {
		if weight < 0 {
			warnings = append(warnings,
				"Negative weight for check '"+checkName+"': weights should be positive")
		}
		if weight > 100 {
			warnings = append(warnings,
				"Very high weight for check '"+checkName+"': consider using smaller values")
		}
	}

	return warnings
}

// CalculateRecommendedWeights suggests weights based on check types and configurations
func (s *Scorer) CalculateRecommendedWeights(results map[string]*HealthResult) map[string]float64 {
	recommendations := make(map[string]float64)

	// Default weights based on check patterns
	for checkName := range results {
		weight := 1.0

		// Assign higher weights to critical components (simple heuristics)
		if s.containsKeywords(checkName, []string{"database", "db", "sql"}) {
			weight = 3.0
		} else if s.containsKeywords(checkName, []string{"api", "service", "endpoint"}) {
			weight = 2.0
		} else if s.containsKeywords(checkName, []string{"cache", "redis", "memcache"}) {
			weight = 1.5
		} else if s.containsKeywords(checkName, []string{"metrics", "monitoring", "health"}) {
			weight = 1.0
		}

		recommendations[checkName] = weight
	}

	return recommendations
}

// containsKeywords checks if a string contains any of the given keywords
func (s *Scorer) containsKeywords(text string, keywords []string) bool {
	for _, keyword := range keywords {
		if len(text) >= len(keyword) {
			for i := 0; i <= len(text)-len(keyword); i++ {
				if text[i:i+len(keyword)] == keyword {
					return true
				}
			}
		}
	}
	return false
}
