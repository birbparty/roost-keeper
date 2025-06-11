package composite

import (
	"fmt"
	"math"
	"sync"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// AnomalyDetector provides statistical anomaly detection for health check patterns
type AnomalyDetector struct {
	logger         *zap.Logger
	historyManager *HistoryManager
}

// NewAnomalyDetector creates a new anomaly detector
func NewAnomalyDetector(logger *zap.Logger) *AnomalyDetector {
	return &AnomalyDetector{
		logger:         logger.With(zap.String("component", "anomaly_detector")),
		historyManager: NewHistoryManager(),
	}
}

// AnomalyType defines different types of anomalies
type AnomalyType string

const (
	// ResponseTimeAnomaly indicates unusual response times
	ResponseTimeAnomaly AnomalyType = "response_time"
	// FailureRateAnomaly indicates unusual failure patterns
	FailureRateAnomaly AnomalyType = "failure_rate"
	// TrendAnomaly indicates unusual trends in health patterns
	TrendAnomaly AnomalyType = "trend"
	// PatternAnomaly indicates unusual patterns in health check sequences
	PatternAnomaly AnomalyType = "pattern"
)

// Anomaly represents a detected anomaly
type Anomaly struct {
	Type        AnomalyType `json:"type"`
	CheckName   string      `json:"check_name"`
	Severity    float64     `json:"severity"`
	Description string      `json:"description"`
	Timestamp   time.Time   `json:"timestamp"`
	Metrics     interface{} `json:"metrics"`
	Confidence  float64     `json:"confidence"`
}

// AnomalyThresholds defines thresholds for anomaly detection
type AnomalyThresholds struct {
	ResponseTimeMultiplier float64 `json:"response_time_multiplier"`
	FailureRateThreshold   float64 `json:"failure_rate_threshold"`
	TrendDeviationFactor   float64 `json:"trend_deviation_factor"`
	MinSampleSize          int     `json:"min_sample_size"`
	ConfidenceThreshold    float64 `json:"confidence_threshold"`
}

// DefaultAnomalyThresholds returns default thresholds
func DefaultAnomalyThresholds() AnomalyThresholds {
	return AnomalyThresholds{
		ResponseTimeMultiplier: 2.0,
		FailureRateThreshold:   0.2,
		TrendDeviationFactor:   2.5,
		MinSampleSize:          10,
		ConfidenceThreshold:    0.8,
	}
}

// DetectAnomalies analyzes health check results for anomalies
func (ad *AnomalyDetector) DetectAnomalies(results map[string]*HealthResult, roost *roostv1alpha1.ManagedRoost) map[string]interface{} {
	if len(results) == 0 {
		return nil
	}

	// Record current results in history
	ad.historyManager.RecordResults(results, roost.Name)

	// Get historical data for analysis
	history := ad.historyManager.GetHistory(roost.Name, 100) // Last 100 results

	thresholds := DefaultAnomalyThresholds()
	var anomalies []Anomaly

	// Analyze each health check for anomalies
	for checkName, currentResult := range results {
		checkHistory := ad.extractCheckHistory(history, checkName)

		if len(checkHistory) < thresholds.MinSampleSize {
			continue // Not enough data for analysis
		}

		// Detect response time anomalies
		if rtAnomaly := ad.detectResponseTimeAnomaly(checkName, currentResult, checkHistory, thresholds); rtAnomaly != nil {
			anomalies = append(anomalies, *rtAnomaly)
		}

		// Detect failure rate anomalies
		if frAnomaly := ad.detectFailureRateAnomaly(checkName, currentResult, checkHistory, thresholds); frAnomaly != nil {
			anomalies = append(anomalies, *frAnomaly)
		}

		// Detect trend anomalies
		if trendAnomaly := ad.detectTrendAnomaly(checkName, currentResult, checkHistory, thresholds); trendAnomaly != nil {
			anomalies = append(anomalies, *trendAnomaly)
		}
	}

	// Detect pattern anomalies across all checks
	if patternAnomaly := ad.detectPatternAnomalies(results, history, thresholds); patternAnomaly != nil {
		anomalies = append(anomalies, *patternAnomaly)
	}

	if len(anomalies) == 0 {
		return nil
	}

	// Filter anomalies by confidence threshold
	filteredAnomalies := ad.filterAnomaliesByConfidence(anomalies, thresholds.ConfidenceThreshold)

	if len(filteredAnomalies) == 0 {
		return nil
	}

	// Convert to map format for return
	anomalyMap := make(map[string]interface{})
	anomalyMap["anomalies"] = filteredAnomalies
	anomalyMap["total_count"] = len(filteredAnomalies)
	anomalyMap["detection_time"] = time.Now()
	anomalyMap["analysis_window"] = len(history)

	ad.logger.Warn("Anomalies detected",
		zap.Int("count", len(filteredAnomalies)),
		zap.String("roost", roost.Name))

	return anomalyMap
}

// detectResponseTimeAnomaly detects unusual response times
func (ad *AnomalyDetector) detectResponseTimeAnomaly(checkName string, current *HealthResult, history []*HealthResult, thresholds AnomalyThresholds) *Anomaly {
	if current.ResponseTime == 0 {
		return nil
	}

	// Calculate statistics from history
	responseTimes := make([]float64, 0, len(history))
	for _, result := range history {
		if result.ResponseTime > 0 {
			responseTimes = append(responseTimes, result.ResponseTime.Seconds())
		}
	}

	if len(responseTimes) < thresholds.MinSampleSize {
		return nil
	}

	mean := calculateMean(responseTimes)
	stdDev := calculateStdDev(responseTimes, mean)
	currentTime := current.ResponseTime.Seconds()

	// Check if current response time is anomalous
	if stdDev > 0 {
		zScore := math.Abs(currentTime-mean) / stdDev

		if zScore > thresholds.ResponseTimeMultiplier {
			severity := math.Min(zScore/thresholds.ResponseTimeMultiplier, 10.0)
			confidence := math.Min(zScore/5.0, 1.0)

			return &Anomaly{
				Type:        ResponseTimeAnomaly,
				CheckName:   checkName,
				Severity:    severity,
				Description: fmt.Sprintf("Response time %.2fs is %.1fx higher than average (%.2fs ± %.2fs)", currentTime, zScore, mean, stdDev),
				Timestamp:   time.Now(),
				Confidence:  confidence,
				Metrics: map[string]interface{}{
					"current_time": currentTime,
					"mean_time":    mean,
					"std_dev":      stdDev,
					"z_score":      zScore,
				},
			}
		}
	}

	return nil
}

// detectFailureRateAnomaly detects unusual failure patterns
func (ad *AnomalyDetector) detectFailureRateAnomaly(checkName string, current *HealthResult, history []*HealthResult, thresholds AnomalyThresholds) *Anomaly {
	windowSize := 10 // Look at last 10 results
	if len(history) < windowSize {
		return nil
	}

	// Calculate recent failure rate
	recentHistory := history[len(history)-windowSize:]
	recentFailures := 0
	for _, result := range recentHistory {
		if !result.Healthy {
			recentFailures++
		}
	}

	recentFailureRate := float64(recentFailures) / float64(windowSize)

	// Calculate historical failure rate (excluding recent window)
	if len(history) < windowSize*2 {
		return nil
	}

	historicalHistory := history[:len(history)-windowSize]
	historicalFailures := 0
	for _, result := range historicalHistory {
		if !result.Healthy {
			historicalFailures++
		}
	}

	historicalFailureRate := float64(historicalFailures) / float64(len(historicalHistory))

	// Check for significant increase in failure rate
	if recentFailureRate > thresholds.FailureRateThreshold && recentFailureRate > historicalFailureRate*2 {
		severity := recentFailureRate * 10
		confidence := math.Min((recentFailureRate-historicalFailureRate)*5, 1.0)

		return &Anomaly{
			Type:        FailureRateAnomaly,
			CheckName:   checkName,
			Severity:    severity,
			Description: fmt.Sprintf("Failure rate increased to %.1f%% (was %.1f%%)", recentFailureRate*100, historicalFailureRate*100),
			Timestamp:   time.Now(),
			Confidence:  confidence,
			Metrics: map[string]interface{}{
				"recent_failure_rate":     recentFailureRate,
				"historical_failure_rate": historicalFailureRate,
				"recent_failures":         recentFailures,
				"window_size":             windowSize,
			},
		}
	}

	return nil
}

// detectTrendAnomaly detects unusual trends in health patterns
func (ad *AnomalyDetector) detectTrendAnomaly(checkName string, current *HealthResult, history []*HealthResult, thresholds AnomalyThresholds) *Anomaly {
	if len(history) < 20 {
		return nil
	}

	// Calculate moving averages to detect trends
	shortWindow := 5
	longWindow := 15

	if len(history) < longWindow {
		return nil
	}

	// Calculate success rates for different windows
	shortTermSuccessRate := ad.calculateSuccessRate(history[len(history)-shortWindow:])
	longTermSuccessRate := ad.calculateSuccessRate(history[len(history)-longWindow:])

	// Check for significant divergence
	divergence := math.Abs(shortTermSuccessRate - longTermSuccessRate)

	if divergence > 0.3 { // 30% divergence threshold
		severity := divergence * 10
		confidence := math.Min(divergence*2, 1.0)

		direction := "improving"
		if shortTermSuccessRate < longTermSuccessRate {
			direction = "degrading"
		}

		return &Anomaly{
			Type:        TrendAnomaly,
			CheckName:   checkName,
			Severity:    severity,
			Description: fmt.Sprintf("Health trend is %s: short-term success rate %.1f%% vs long-term %.1f%%", direction, shortTermSuccessRate*100, longTermSuccessRate*100),
			Timestamp:   time.Now(),
			Confidence:  confidence,
			Metrics: map[string]interface{}{
				"short_term_success_rate": shortTermSuccessRate,
				"long_term_success_rate":  longTermSuccessRate,
				"divergence":              divergence,
				"direction":               direction,
			},
		}
	}

	return nil
}

// detectPatternAnomalies detects unusual patterns across all health checks
func (ad *AnomalyDetector) detectPatternAnomalies(results map[string]*HealthResult, history []*HistoryEntry, thresholds AnomalyThresholds) *Anomaly {
	if len(history) < thresholds.MinSampleSize {
		return nil
	}

	// Calculate current overall failure rate
	currentFailures := 0
	for _, result := range results {
		if !result.Healthy {
			currentFailures++
		}
	}
	currentFailureRate := float64(currentFailures) / float64(len(results))

	// Calculate historical overall failure rates
	historicalRates := make([]float64, 0, len(history))
	for _, entry := range history {
		failures := 0
		for _, result := range entry.Results {
			if !result.Healthy {
				failures++
			}
		}
		rate := float64(failures) / float64(len(entry.Results))
		historicalRates = append(historicalRates, rate)
	}

	if len(historicalRates) == 0 {
		return nil
	}

	meanRate := calculateMean(historicalRates)
	stdDev := calculateStdDev(historicalRates, meanRate)

	if stdDev > 0 {
		zScore := math.Abs(currentFailureRate-meanRate) / stdDev

		if zScore > thresholds.TrendDeviationFactor {
			severity := math.Min(zScore/thresholds.TrendDeviationFactor*5, 10.0)
			confidence := math.Min(zScore/5.0, 1.0)

			return &Anomaly{
				Type:        PatternAnomaly,
				CheckName:   "overall",
				Severity:    severity,
				Description: fmt.Sprintf("Overall failure pattern anomaly: current rate %.1f%% vs historical %.1f%% ± %.1f%%", currentFailureRate*100, meanRate*100, stdDev*100),
				Timestamp:   time.Now(),
				Confidence:  confidence,
				Metrics: map[string]interface{}{
					"current_failure_rate": currentFailureRate,
					"historical_mean_rate": meanRate,
					"historical_std_dev":   stdDev,
					"z_score":              zScore,
				},
			}
		}
	}

	return nil
}

// Helper functions

func (ad *AnomalyDetector) extractCheckHistory(history []*HistoryEntry, checkName string) []*HealthResult {
	var checkHistory []*HealthResult

	for _, entry := range history {
		if result, exists := entry.Results[checkName]; exists {
			checkHistory = append(checkHistory, result)
		}
	}

	return checkHistory
}

func (ad *AnomalyDetector) calculateSuccessRate(results []*HealthResult) float64 {
	if len(results) == 0 {
		return 0.0
	}

	successes := 0
	for _, result := range results {
		if result.Healthy {
			successes++
		}
	}

	return float64(successes) / float64(len(results))
}

func (ad *AnomalyDetector) filterAnomaliesByConfidence(anomalies []Anomaly, threshold float64) []Anomaly {
	var filtered []Anomaly

	for _, anomaly := range anomalies {
		if anomaly.Confidence >= threshold {
			filtered = append(filtered, anomaly)
		}
	}

	return filtered
}

// Statistical helper functions

func calculateMean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}

	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) <= 1 {
		return 0
	}

	sumSquaredDiffs := 0.0
	for _, v := range values {
		diff := v - mean
		sumSquaredDiffs += diff * diff
	}

	variance := sumSquaredDiffs / float64(len(values)-1)
	return math.Sqrt(variance)
}

// HistoryManager manages historical health check data
type HistoryManager struct {
	history map[string][]*HistoryEntry
	mutex   sync.RWMutex
}

// HistoryEntry represents a historical health check result set
type HistoryEntry struct {
	Timestamp time.Time                `json:"timestamp"`
	Results   map[string]*HealthResult `json:"results"`
}

func NewHistoryManager() *HistoryManager {
	return &HistoryManager{
		history: make(map[string][]*HistoryEntry),
	}
}

func (hm *HistoryManager) RecordResults(results map[string]*HealthResult, roostName string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	entry := &HistoryEntry{
		Timestamp: time.Now(),
		Results:   make(map[string]*HealthResult),
	}

	// Deep copy results
	for checkName, result := range results {
		entry.Results[checkName] = &HealthResult{
			Healthy:      result.Healthy,
			Message:      result.Message,
			ResponseTime: result.ResponseTime,
			CheckTime:    result.CheckTime,
			FailureCount: result.FailureCount,
			LastError:    result.LastError,
		}
	}

	if _, exists := hm.history[roostName]; !exists {
		hm.history[roostName] = make([]*HistoryEntry, 0)
	}

	hm.history[roostName] = append(hm.history[roostName], entry)

	// Limit history size to prevent memory issues
	maxHistorySize := 1000
	if len(hm.history[roostName]) > maxHistorySize {
		// Keep only the most recent entries
		hm.history[roostName] = hm.history[roostName][len(hm.history[roostName])-maxHistorySize:]
	}
}

func (hm *HistoryManager) GetHistory(roostName string, limit int) []*HistoryEntry {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	entries, exists := hm.history[roostName]
	if !exists {
		return []*HistoryEntry{}
	}

	if limit <= 0 || limit >= len(entries) {
		// Return a copy to avoid concurrent modification
		result := make([]*HistoryEntry, len(entries))
		copy(result, entries)
		return result
	}

	// Return the most recent 'limit' entries
	start := len(entries) - limit
	result := make([]*HistoryEntry, limit)
	copy(result, entries[start:])
	return result
}

func (hm *HistoryManager) ClearHistory(roostName string) {
	hm.mutex.Lock()
	defer hm.mutex.Unlock()

	delete(hm.history, roostName)
}

func (hm *HistoryManager) GetHistorySize(roostName string) int {
	hm.mutex.RLock()
	defer hm.mutex.RUnlock()

	if entries, exists := hm.history[roostName]; exists {
		return len(entries)
	}
	return 0
}
