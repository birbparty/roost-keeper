package api

import (
	"context"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/record"

	roostkeeper "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// LifecycleTracker tracks the complete lifecycle of ManagedRoost resources
type LifecycleTracker struct {
	logger   *zap.Logger
	recorder record.EventRecorder
}

// NewLifecycleTracker creates a new lifecycle tracker
func NewLifecycleTracker(logger *zap.Logger, recorder record.EventRecorder) *LifecycleTracker {
	return &LifecycleTracker{
		logger:   logger,
		recorder: recorder,
	}
}

// TrackEvent records a lifecycle event for a ManagedRoost resource
func (lt *LifecycleTracker) TrackEvent(ctx context.Context, managedRoost *roostkeeper.ManagedRoost, event LifecycleEvent) error {
	log := lt.logger.With(
		zap.String("operation", "track_event"),
		zap.String("event_type", string(event.Type)),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Recording lifecycle event", zap.String("message", event.Message))

	// Initialize lifecycle status if not present
	if managedRoost.Status.Lifecycle == nil {
		managedRoost.Status.Lifecycle = &roostkeeper.LifecycleStatus{}
	}

	// Update lifecycle tracking
	lt.updateLifecycleStatus(managedRoost.Status.Lifecycle, event)

	// Emit Kubernetes event
	eventType := event.EventType
	if eventType == "" {
		eventType = "Normal"
	}

	lt.recorder.Event(managedRoost, eventType, string(event.Type), event.Message)

	log.Info("Lifecycle event recorded successfully")
	return nil
}

// updateLifecycleStatus updates the lifecycle status with the new event
func (lt *LifecycleTracker) updateLifecycleStatus(lifecycle *roostkeeper.LifecycleStatus, event LifecycleEvent) {
	now := metav1.Time{Time: time.Now()}

	// Update creation time if this is the first event
	if lifecycle.CreatedAt == nil && event.Type == LifecycleEventCreated {
		lifecycle.CreatedAt = &now
	}

	// Update last activity
	lifecycle.LastActivity = &now

	// Add lifecycle event
	lifecycleEvent := roostkeeper.LifecycleEventRecord{
		Type:      string(event.Type),
		Timestamp: now,
		Message:   event.Message,
		Details:   event.Details,
	}

	lifecycle.Events = append(lifecycle.Events, lifecycleEvent)

	// Limit lifecycle events to prevent unbounded growth
	maxEvents := 50
	if len(lifecycle.Events) > maxEvents {
		lifecycle.Events = lifecycle.Events[len(lifecycle.Events)-maxEvents:]
	}

	// Update event counters
	if lifecycle.EventCounts == nil {
		lifecycle.EventCounts = make(map[string]int32)
	}
	lifecycle.EventCounts[string(event.Type)]++

	// Update phase durations - this would be enhanced with actual phase tracking
	if lifecycle.PhaseDurations == nil {
		lifecycle.PhaseDurations = make(map[string]metav1.Duration)
	}
	// For now, we'll implement basic phase duration tracking
	// This could be enhanced to track actual time spent in each phase
}

// GetLifecycleHistory returns the complete lifecycle history for a resource
func (lt *LifecycleTracker) GetLifecycleHistory(managedRoost *roostkeeper.ManagedRoost) *roostkeeper.LifecycleStatus {
	if managedRoost.Status.Lifecycle == nil {
		return &roostkeeper.LifecycleStatus{}
	}
	return managedRoost.Status.Lifecycle
}

// GetEventCount returns the count of a specific event type
func (lt *LifecycleTracker) GetEventCount(managedRoost *roostkeeper.ManagedRoost, eventType LifecycleEventType) int32 {
	if managedRoost.Status.Lifecycle == nil || managedRoost.Status.Lifecycle.EventCounts == nil {
		return 0
	}
	return managedRoost.Status.Lifecycle.EventCounts[string(eventType)]
}

// GetRecentEvents returns the most recent lifecycle events
func (lt *LifecycleTracker) GetRecentEvents(managedRoost *roostkeeper.ManagedRoost, limit int) []roostkeeper.LifecycleEventRecord {
	if managedRoost.Status.Lifecycle == nil || len(managedRoost.Status.Lifecycle.Events) == 0 {
		return []roostkeeper.LifecycleEventRecord{}
	}

	events := managedRoost.Status.Lifecycle.Events
	start := len(events) - limit
	if start < 0 {
		start = 0
	}

	return events[start:]
}

// IsEventRecent checks if an event type has occurred recently within the specified duration
func (lt *LifecycleTracker) IsEventRecent(managedRoost *roostkeeper.ManagedRoost, eventType LifecycleEventType, within time.Duration) bool {
	if managedRoost.Status.Lifecycle == nil || len(managedRoost.Status.Lifecycle.Events) == 0 {
		return false
	}

	cutoff := time.Now().Add(-within)

	// Search from the end (most recent) backwards
	for i := len(managedRoost.Status.Lifecycle.Events) - 1; i >= 0; i-- {
		event := managedRoost.Status.Lifecycle.Events[i]
		if event.Type == string(eventType) && event.Timestamp.Time.After(cutoff) {
			return true
		}
		// If we've gone past the cutoff time, no need to search further
		if event.Timestamp.Time.Before(cutoff) {
			break
		}
	}

	return false
}
