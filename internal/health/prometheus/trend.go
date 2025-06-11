package prometheus

import (
	"context"
	"fmt"
	"math"
	"time"

	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// TrendResult represents the result of trend analysis on time-series data
type TrendResult struct {
	IsImproving bool    `json:"is_improving"`
	IsDegrading bool    `json:"is_degrading"`
	IsStable    bool    `json:"is_stable"`
	Slope       float64 `json:"slope"`       // Linear regression slope
	Correlation float64 `json:"correlation"` // Correlation coefficient (R)
	DataPoints  int     `json:"data_points"` // Number of data points analyzed
	TimeWindow  string  `json:"time_window"` // Time window analyzed
	Confidence  float64 `json:"confidence"`  // Confidence in the trend (0-1)
}

// analyzeTrend performs trend analysis on the given query over a time window
func (p *PrometheusChecker) analyzeTrend(ctx context.Context, client v1.API, query string, trendConfig *roostv1alpha1.TrendAnalysisSpec) (*TrendResult, error) {
	// Get time window
	timeWindow := 15 * time.Minute
	if trendConfig.TimeWindow.Duration > 0 {
		timeWindow = trendConfig.TimeWindow.Duration
	}

	// Execute range query to get historical data
	end := time.Now()
	start := end.Add(-timeWindow)
	step := timeWindow / 20 // 20 data points

	rangeQuery := v1.Range{
		Start: start,
		End:   end,
		Step:  step,
	}

	value, warnings, err := client.QueryRange(ctx, query, rangeQuery)
	if err != nil {
		return nil, fmt.Errorf("failed to execute range query for trend analysis: %w", err)
	}

	// Log warnings
	if len(warnings) > 0 {
		p.logger.Debug("Range query warnings",
			zap.Strings("warnings", []string(warnings)),
			zap.String("query", query),
		)
	}

	// Parse and analyze the time series data
	matrix, ok := value.(model.Matrix)
	if !ok {
		return nil, fmt.Errorf("expected matrix result for trend analysis, got %T", value)
	}

	if len(matrix) == 0 {
		return &TrendResult{
			IsStable:   true,
			DataPoints: 0,
			TimeWindow: timeWindow.String(),
			Confidence: 0.0,
		}, nil
	}

	// Use the first series for trend analysis
	series := matrix[0]
	if len(series.Values) < 2 {
		return &TrendResult{
			IsStable:   true,
			DataPoints: len(series.Values),
			TimeWindow: timeWindow.String(),
			Confidence: 0.0,
		}, nil
	}

	// Perform linear regression analysis
	slope, correlation := p.calculateLinearRegression(series.Values)

	// Determine trend direction based on slope and thresholds
	improvementThreshold := float64(trendConfig.ImprovementThreshold) / 100.0
	degradationThreshold := float64(trendConfig.DegradationThreshold) / 100.0

	// Calculate trend confidence based on correlation strength
	confidence := math.Abs(correlation)

	// Determine trend classification
	var isImproving, isDegrading, isStable bool

	// Use normalized slope for trend detection
	dataRange := p.calculateDataRange(series.Values)
	normalizedSlope := slope / dataRange * 100 // Slope as percentage of range

	if math.Abs(normalizedSlope) < improvementThreshold {
		isStable = true
	} else if normalizedSlope > improvementThreshold {
		isImproving = true
	} else if normalizedSlope < -degradationThreshold {
		isDegrading = true
	} else {
		isStable = true
	}

	// Require minimum confidence for trend classification
	minConfidence := 0.3
	if confidence < minConfidence {
		isImproving = false
		isDegrading = false
		isStable = true
	}

	return &TrendResult{
		IsImproving: isImproving,
		IsDegrading: isDegrading,
		IsStable:    isStable,
		Slope:       slope,
		Correlation: correlation,
		DataPoints:  len(series.Values),
		TimeWindow:  timeWindow.String(),
		Confidence:  confidence,
	}, nil
}

// calculateLinearRegression calculates the slope and correlation coefficient for the given data points
func (p *PrometheusChecker) calculateLinearRegression(values []model.SamplePair) (slope, correlation float64) {
	n := len(values)
	if n < 2 {
		return 0, 0
	}

	var sumX, sumY, sumXY, sumX2, sumY2 float64

	for i, value := range values {
		x := float64(i) // Use index as x value for simplicity
		y := float64(value.Value)

		sumX += x
		sumY += y
		sumXY += x * y
		sumX2 += x * x
		sumY2 += y * y
	}

	nf := float64(n)

	// Calculate slope (b1) and correlation coefficient (r)
	numerator := nf*sumXY - sumX*sumY
	denominatorSlope := nf*sumX2 - sumX*sumX
	denominatorCorr := math.Sqrt((nf*sumX2 - sumX*sumX) * (nf*sumY2 - sumY*sumY))

	if denominatorSlope == 0 {
		slope = 0
	} else {
		slope = numerator / denominatorSlope
	}

	if denominatorCorr == 0 {
		correlation = 0
	} else {
		correlation = numerator / denominatorCorr
	}

	return slope, correlation
}

// calculateDataRange calculates the range (max - min) of the data points
func (p *PrometheusChecker) calculateDataRange(values []model.SamplePair) float64 {
	if len(values) == 0 {
		return 1.0 // Avoid division by zero
	}

	min := float64(values[0].Value)
	max := float64(values[0].Value)

	for _, value := range values {
		v := float64(value.Value)
		if v < min {
			min = v
		}
		if v > max {
			max = v
		}
	}

	dataRange := max - min
	if dataRange == 0 {
		return 1.0 // Avoid division by zero for constant values
	}

	return dataRange
}

// calculateMovingAverage calculates a simple moving average for smoothing
func (p *PrometheusChecker) calculateMovingAverage(values []model.SamplePair, window int) []float64 {
	if window <= 0 || len(values) == 0 {
		return nil
	}

	result := make([]float64, len(values))

	for i := range values {
		start := i - window/2
		end := i + window/2 + 1

		if start < 0 {
			start = 0
		}
		if end > len(values) {
			end = len(values)
		}

		sum := 0.0
		count := 0
		for j := start; j < end; j++ {
			sum += float64(values[j].Value)
			count++
		}

		if count > 0 {
			result[i] = sum / float64(count)
		} else {
			result[i] = float64(values[i].Value)
		}
	}

	return result
}
