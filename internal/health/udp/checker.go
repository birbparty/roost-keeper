package udp

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"go.uber.org/zap"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// ExecuteUDPCheck performs a UDP health check
func ExecuteUDPCheck(ctx context.Context, logger *zap.Logger, roost *roostv1alpha1.ManagedRoost, checkSpec roostv1alpha1.HealthCheckSpec, udpSpec roostv1alpha1.UDPHealthCheckSpec) (bool, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "health.udp_check", roost.Name, roost.Namespace)
	defer span.End()

	log := logger.With(
		zap.String("check_name", checkSpec.Name),
		zap.String("host", udpSpec.Host),
		zap.Int32("port", udpSpec.Port),
	)

	log.Debug("Starting UDP health check")

	// Determine timeouts
	timeout := 10 * time.Second
	if checkSpec.Timeout.Duration > 0 {
		timeout = checkSpec.Timeout.Duration
	}

	readTimeout := 5 * time.Second
	if udpSpec.ReadTimeout != nil && udpSpec.ReadTimeout.Duration > 0 {
		readTimeout = udpSpec.ReadTimeout.Duration
	}

	// Determine retry count
	retries := int32(3)
	if udpSpec.Retries > 0 {
		retries = udpSpec.Retries
	}

	// Construct address
	address := fmt.Sprintf("%s:%d", udpSpec.Host, udpSpec.Port)

	log.Debug("Attempting UDP communication",
		zap.String("address", address),
		zap.Duration("timeout", timeout),
		zap.Duration("read_timeout", readTimeout),
		zap.Int32("retries", retries),
	)

	// Set overall deadline for the check
	checkCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Resolve UDP address
	udpAddr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		telemetry.RecordSpanError(ctx, err)
		log.Error("Failed to resolve UDP address", zap.Error(err))
		return false, fmt.Errorf("failed to resolve UDP address: %w", err)
	}

	// Attempt UDP communication with retries
	var lastErr error
	for attempt := int32(1); attempt <= retries; attempt++ {
		log.Debug("UDP attempt", zap.Int32("attempt", attempt), zap.Int32("max_retries", retries))

		// Check if context is cancelled
		select {
		case <-checkCtx.Done():
			telemetry.RecordSpanError(ctx, checkCtx.Err())
			return false, fmt.Errorf("UDP check timed out: %w", checkCtx.Err())
		default:
		}

		success, err := performUDPCheck(checkCtx, log, udpAddr, udpSpec, readTimeout)
		if err == nil && success {
			log.Debug("UDP health check passed", zap.Int32("attempt", attempt))
			telemetry.RecordSpanSuccess(ctx)
			return true, nil
		}

		lastErr = err
		if err != nil {
			log.Debug("UDP attempt failed",
				zap.Int32("attempt", attempt),
				zap.Error(err),
			)
		}

		// Wait before retry (except on last attempt)
		if attempt < retries {
			retryDelay := time.Duration(attempt) * 100 * time.Millisecond
			log.Debug("Waiting before retry", zap.Duration("delay", retryDelay))

			select {
			case <-checkCtx.Done():
				telemetry.RecordSpanError(ctx, checkCtx.Err())
				return false, fmt.Errorf("UDP check timed out during retry: %w", checkCtx.Err())
			case <-time.After(retryDelay):
				// Continue to next retry
			}
		}
	}

	// All attempts failed
	telemetry.RecordSpanError(ctx, lastErr)
	log.Error("UDP health check failed after all retries",
		zap.Int32("retries", retries),
		zap.Error(lastErr),
	)
	return false, fmt.Errorf("UDP check failed after %d attempts: %w", retries, lastErr)
}

// performUDPCheck performs a single UDP check attempt
func performUDPCheck(ctx context.Context, log *zap.Logger, udpAddr *net.UDPAddr, udpSpec roostv1alpha1.UDPHealthCheckSpec, readTimeout time.Duration) (bool, error) {
	// Create UDP connection
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return false, fmt.Errorf("UDP connection failed: %w", err)
	}
	defer conn.Close()

	// Set read deadline
	if err := conn.SetReadDeadline(time.Now().Add(readTimeout)); err != nil {
		return false, fmt.Errorf("failed to set read deadline: %w", err)
	}

	// Determine what data to send
	sendData := "ping"
	if udpSpec.SendData != "" {
		sendData = udpSpec.SendData
	}

	log.Debug("Sending UDP data", zap.String("data", sendData))

	// Send UDP packet
	_, err = conn.Write([]byte(sendData))
	if err != nil {
		return false, fmt.Errorf("failed to send UDP data: %w", err)
	}

	// If no response is expected, consider successful send as success
	if udpSpec.ExpectedResponse == "" {
		log.Debug("No response expected, UDP send successful")
		return true, nil
	}

	// Read response
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return false, fmt.Errorf("UDP read timeout waiting for response")
		}
		return false, fmt.Errorf("failed to read UDP response: %w", err)
	}

	response := string(buffer[:n])
	log.Debug("Received UDP response",
		zap.String("response", response),
		zap.String("expected", udpSpec.ExpectedResponse),
	)

	// Validate response
	if !validateUDPResponse(response, udpSpec.ExpectedResponse) {
		return false, fmt.Errorf("unexpected UDP response: got '%s', expected '%s'", response, udpSpec.ExpectedResponse)
	}

	log.Debug("UDP response validation passed")
	return true, nil
}

// validateUDPResponse validates if the received response matches expectations
func validateUDPResponse(response, expected string) bool {
	// Exact match
	if response == expected {
		return true
	}

	// Case-insensitive match
	if strings.EqualFold(response, expected) {
		return true
	}

	// Contains match (for flexible protocol responses)
	if strings.Contains(response, expected) {
		return true
	}

	return false
}
