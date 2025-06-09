package helm

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
	"github.com/birbparty/roost-keeper/internal/telemetry"
)

// prepareValues prepares Helm chart values from the ManagedRoost specification
func (hm *HelmManager) prepareValues(ctx context.Context, roost *roostv1alpha1.ManagedRoost) (map[string]interface{}, error) {
	ctx, span := telemetry.StartControllerSpan(ctx, "helm.prepare_values", roost.Name, roost.Namespace)
	defer span.End()

	log := hm.Logger.With(zap.String("operation", "prepare_values"))

	values := make(map[string]interface{})

	// If no values configuration is provided, return empty values
	if roost.Spec.Chart.Values == nil {
		return values, nil
	}

	valuesSpec := roost.Spec.Chart.Values

	// Process inline values first
	if valuesSpec.Inline != "" {
		log.Info("Processing inline values")

		inlineValues := make(map[string]interface{})
		if err := yaml.Unmarshal([]byte(valuesSpec.Inline), &inlineValues); err != nil {
			telemetry.RecordSpanError(ctx, err)
			return nil, fmt.Errorf("failed to parse inline values: %w", err)
		}

		// Merge inline values into base values
		values = mergeMaps(values, inlineValues)
	}

	// Process ConfigMap references
	if len(valuesSpec.ConfigMapRefs) > 0 {
		log.Info("Processing ConfigMap references", zap.Int("count", len(valuesSpec.ConfigMapRefs)))

		for _, ref := range valuesSpec.ConfigMapRefs {
			configMapValues, err := hm.loadValuesFromConfigMap(ctx, roost, ref)
			if err != nil {
				telemetry.RecordSpanError(ctx, err)
				return nil, fmt.Errorf("failed to load values from ConfigMap %s: %w", ref.Name, err)
			}
			values = mergeMaps(values, configMapValues)
		}
	}

	// Process Secret references
	if len(valuesSpec.SecretRefs) > 0 {
		log.Info("Processing Secret references", zap.Int("count", len(valuesSpec.SecretRefs)))

		for _, ref := range valuesSpec.SecretRefs {
			secretValues, err := hm.loadValuesFromSecret(ctx, roost, ref)
			if err != nil {
				telemetry.RecordSpanError(ctx, err)
				return nil, fmt.Errorf("failed to load values from Secret %s: %w", ref.Name, err)
			}
			values = mergeMaps(values, secretValues)
		}
	}

	// Process template if enabled
	if valuesSpec.Template != nil && valuesSpec.Template.Enabled {
		log.Info("Processing value templates")

		templatedValues, err := hm.processValueTemplates(ctx, roost, values, valuesSpec.Template)
		if err != nil {
			telemetry.RecordSpanError(ctx, err)
			return nil, fmt.Errorf("failed to process value templates: %w", err)
		}
		values = templatedValues
	}

	log.Info("Values prepared successfully", zap.Int("value_count", len(values)))
	telemetry.RecordSpanSuccess(ctx)
	return values, nil
}

// loadValuesFromConfigMap loads values from a ConfigMap
func (hm *HelmManager) loadValuesFromConfigMap(ctx context.Context, roost *roostv1alpha1.ManagedRoost, ref roostv1alpha1.ValueSourceRef) (map[string]interface{}, error) {
	log := hm.Logger.With(
		zap.String("configmap", ref.Name),
		zap.String("key", ref.Key),
	)

	// Determine namespace
	namespace := ref.Namespace
	if namespace == "" {
		namespace = roost.Namespace
	}

	// Fetch ConfigMap
	var configMap corev1.ConfigMap
	namespacedName := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}

	if err := hm.Client.Get(ctx, namespacedName, &configMap); err != nil {
		return nil, fmt.Errorf("failed to get ConfigMap %s: %w", namespacedName, err)
	}

	values := make(map[string]interface{})

	// If specific key is requested, only process that key
	if ref.Key != "" {
		data, exists := configMap.Data[ref.Key]
		if !exists {
			return nil, fmt.Errorf("key %s not found in ConfigMap %s", ref.Key, ref.Name)
		}

		if err := yaml.Unmarshal([]byte(data), &values); err != nil {
			return nil, fmt.Errorf("failed to parse YAML from ConfigMap key %s: %w", ref.Key, err)
		}
	} else {
		// Process all keys in the ConfigMap
		for key, data := range configMap.Data {
			var keyValues map[string]interface{}
			if err := yaml.Unmarshal([]byte(data), &keyValues); err != nil {
				log.Warn("Failed to parse YAML from ConfigMap key, treating as string",
					zap.String("key", key),
					zap.Error(err),
				)
				values[key] = data
				continue
			}
			values = mergeMaps(values, keyValues)
		}
	}

	log.Info("Loaded values from ConfigMap", zap.Int("value_count", len(values)))
	return values, nil
}

// loadValuesFromSecret loads values from a Secret
func (hm *HelmManager) loadValuesFromSecret(ctx context.Context, roost *roostv1alpha1.ManagedRoost, ref roostv1alpha1.ValueSourceRef) (map[string]interface{}, error) {
	log := hm.Logger.With(
		zap.String("secret", ref.Name),
		zap.String("key", ref.Key),
	)

	// Determine namespace
	namespace := ref.Namespace
	if namespace == "" {
		namespace = roost.Namespace
	}

	// Fetch Secret
	var secret corev1.Secret
	namespacedName := types.NamespacedName{
		Name:      ref.Name,
		Namespace: namespace,
	}

	if err := hm.Client.Get(ctx, namespacedName, &secret); err != nil {
		return nil, fmt.Errorf("failed to get Secret %s: %w", namespacedName, err)
	}

	values := make(map[string]interface{})

	// If specific key is requested, only process that key
	if ref.Key != "" {
		data, exists := secret.Data[ref.Key]
		if !exists {
			return nil, fmt.Errorf("key %s not found in Secret %s", ref.Key, ref.Name)
		}

		if err := yaml.Unmarshal(data, &values); err != nil {
			return nil, fmt.Errorf("failed to parse YAML from Secret key %s: %w", ref.Key, err)
		}
	} else {
		// Process all keys in the Secret
		for key, data := range secret.Data {
			var keyValues map[string]interface{}
			if err := yaml.Unmarshal(data, &keyValues); err != nil {
				log.Warn("Failed to parse YAML from Secret key, treating as string",
					zap.String("key", key),
					zap.Error(err),
				)
				values[key] = string(data)
				continue
			}
			values = mergeMaps(values, keyValues)
		}
	}

	log.Info("Loaded values from Secret", zap.Int("value_count", len(values)))
	return values, nil
}

// processValueTemplates processes template variables in values
func (hm *HelmManager) processValueTemplates(ctx context.Context, roost *roostv1alpha1.ManagedRoost, values map[string]interface{}, template *roostv1alpha1.ValueTemplateSpec) (map[string]interface{}, error) {
	log := hm.Logger.With(zap.String("operation", "template_processing"))

	// Create template context with provided variables
	templateContext := make(map[string]string)
	if template.Context != nil {
		for k, v := range template.Context {
			templateContext[k] = v
		}
	}

	// Add built-in template variables
	templateContext["ROOST_NAME"] = roost.Name
	templateContext["ROOST_NAMESPACE"] = roost.Namespace
	templateContext["TARGET_NAMESPACE"] = hm.getTargetNamespace(roost)

	// Process templates in values
	processedValues, err := hm.processTemplatesInMap(values, templateContext)
	if err != nil {
		return nil, fmt.Errorf("failed to process templates: %w", err)
	}

	log.Info("Template processing completed")
	return processedValues, nil
}

// processTemplatesInMap recursively processes template variables in a map
func (hm *HelmManager) processTemplatesInMap(input map[string]interface{}, context map[string]string) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	for key, value := range input {
		switch v := value.(type) {
		case string:
			// Process template variables in string values
			processed := v
			for placeholder, replacement := range context {
				processed = strings.ReplaceAll(processed, fmt.Sprintf("${%s}", placeholder), replacement)
				processed = strings.ReplaceAll(processed, fmt.Sprintf("{{%s}}", placeholder), replacement)
			}
			result[key] = processed
		case map[string]interface{}:
			// Recursively process nested maps
			nested, err := hm.processTemplatesInMap(v, context)
			if err != nil {
				return nil, err
			}
			result[key] = nested
		case []interface{}:
			// Process arrays
			processed, err := hm.processTemplatesInSlice(v, context)
			if err != nil {
				return nil, err
			}
			result[key] = processed
		default:
			// Keep other types as-is
			result[key] = value
		}
	}

	return result, nil
}

// processTemplatesInSlice processes template variables in a slice
func (hm *HelmManager) processTemplatesInSlice(input []interface{}, context map[string]string) ([]interface{}, error) {
	result := make([]interface{}, len(input))

	for i, value := range input {
		switch v := value.(type) {
		case string:
			// Process template variables in string values
			processed := v
			for placeholder, replacement := range context {
				processed = strings.ReplaceAll(processed, fmt.Sprintf("${%s}", placeholder), replacement)
				processed = strings.ReplaceAll(processed, fmt.Sprintf("{{%s}}", placeholder), replacement)
			}
			result[i] = processed
		case map[string]interface{}:
			// Recursively process nested maps
			nested, err := hm.processTemplatesInMap(v, context)
			if err != nil {
				return nil, err
			}
			result[i] = nested
		case []interface{}:
			// Recursively process nested slices
			nested, err := hm.processTemplatesInSlice(v, context)
			if err != nil {
				return nil, err
			}
			result[i] = nested
		default:
			// Keep other types as-is
			result[i] = value
		}
	}

	return result, nil
}

// mergeMaps merges two maps, with the second map taking precedence
func mergeMaps(base, override map[string]interface{}) map[string]interface{} {
	result := make(map[string]interface{})

	// Copy base map
	for k, v := range base {
		result[k] = v
	}

	// Merge override map
	for k, v := range override {
		if existing, exists := result[k]; exists {
			// If both values are maps, merge them recursively
			if existingMap, ok := existing.(map[string]interface{}); ok {
				if overrideMap, ok := v.(map[string]interface{}); ok {
					result[k] = mergeMaps(existingMap, overrideMap)
					continue
				}
			}
		}
		// Otherwise, override takes precedence
		result[k] = v
	}

	return result
}
