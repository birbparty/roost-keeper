package tenancy

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	roostv1alpha1 "github.com/birbparty/roost-keeper/api/v1alpha1"
)

// NetworkManager handles tenant network isolation through Kubernetes NetworkPolicies
type NetworkManager struct {
	client client.Client
	logger *zap.Logger
}

// NewNetworkManager creates a new network manager
func NewNetworkManager(client client.Client, logger *zap.Logger) *NetworkManager {
	return &NetworkManager{
		client: client,
		logger: logger.With(zap.String("component", "network-manager")),
	}
}

// ApplyNetworkPolicies creates network policies for tenant isolation
func (n *NetworkManager) ApplyNetworkPolicies(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := n.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Applying tenant network policies")

	// Create default deny-all policy
	if err := n.createDefaultDenyPolicy(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create default deny policy: %w", err)
	}

	// Create tenant isolation policy
	if err := n.createTenantIsolationPolicy(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create tenant isolation policy: %w", err)
	}

	// Create DNS access policy
	if err := n.createDNSAccessPolicy(ctx, managedRoost, tenantID); err != nil {
		return fmt.Errorf("failed to create DNS access policy: %w", err)
	}

	// Apply custom network policies if configured
	if managedRoost.Spec.Tenancy.NetworkPolicy != nil {
		if err := n.applyCustomNetworkPolicies(ctx, managedRoost, tenantID); err != nil {
			return fmt.Errorf("failed to apply custom network policies: %w", err)
		}
	}

	log.Info("Tenant network policies applied successfully")
	return nil
}

// CleanupNetworkPolicies removes tenant-specific network policies
func (n *NetworkManager) CleanupNetworkPolicies(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	log := n.logger.With(
		zap.String("tenant", tenantID),
		zap.String("roost", managedRoost.Name),
		zap.String("namespace", managedRoost.Namespace),
	)

	log.Info("Cleaning up tenant network policies")

	var cleanupErrors []error

	// List all network policies with tenant label
	var networkPolicies networkingv1.NetworkPolicyList
	listOpts := []client.ListOption{
		client.InNamespace(managedRoost.Namespace),
		client.MatchingLabels{TenantIDLabel: tenantID},
	}

	if err := n.client.List(ctx, &networkPolicies, listOpts...); err != nil {
		return fmt.Errorf("failed to list network policies: %w", err)
	}

	// Delete each network policy
	for _, policy := range networkPolicies.Items {
		if err := n.client.Delete(ctx, &policy); err != nil && !errors.IsNotFound(err) {
			cleanupErrors = append(cleanupErrors, fmt.Errorf("failed to delete network policy %s: %w", policy.Name, err))
		}
	}

	if len(cleanupErrors) > 0 {
		return fmt.Errorf("network policy cleanup failed with %d errors: %v", len(cleanupErrors), cleanupErrors)
	}

	log.Info("Tenant network policies cleanup completed successfully")
	return nil
}

// createDefaultDenyPolicy creates a default deny-all network policy for the tenant
func (n *NetworkManager) createDefaultDenyPolicy(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	policyName := fmt.Sprintf("roost-keeper-tenant-%s-deny-all", tenantID)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentNetworkPolicy,
				"policy-type":        "default-deny",
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					TenantIDLabel: tenantID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			// Empty ingress and egress rules = deny all by default
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, policy, n.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := n.client.Create(ctx, policy); err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Debug("Default deny policy already exists", zap.String("name", policyName))
			return nil
		}
		return fmt.Errorf("failed to create default deny policy: %w", err)
	}

	n.logger.Info("Created default deny network policy", zap.String("name", policyName))
	return nil
}

// createTenantIsolationPolicy creates a policy allowing communication only within the same tenant
func (n *NetworkManager) createTenantIsolationPolicy(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	policyName := fmt.Sprintf("roost-keeper-tenant-%s-isolation", tenantID)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentNetworkPolicy,
				"policy-type":        "tenant-isolation",
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					TenantIDLabel: tenantID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
				networkingv1.PolicyTypeEgress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									TenantIDLabel: tenantID,
								},
							},
						},
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									TenantIDLabel: tenantID,
								},
							},
						},
					},
				},
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					To: []networkingv1.NetworkPolicyPeer{
						{
							PodSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									TenantIDLabel: tenantID,
								},
							},
						},
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									TenantIDLabel: tenantID,
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, policy, n.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := n.client.Create(ctx, policy); err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Debug("Tenant isolation policy already exists", zap.String("name", policyName))
			return nil
		}
		return fmt.Errorf("failed to create tenant isolation policy: %w", err)
	}

	n.logger.Info("Created tenant isolation network policy", zap.String("name", policyName))
	return nil
}

// createDNSAccessPolicy creates a policy allowing DNS access
func (n *NetworkManager) createDNSAccessPolicy(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	policyName := fmt.Sprintf("roost-keeper-tenant-%s-dns", tenantID)

	dnsPort := intstr.FromInt(53)

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentNetworkPolicy,
				"policy-type":        "dns-access",
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					TenantIDLabel: tenantID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: []networkingv1.NetworkPolicyEgressRule{
				{
					// Allow DNS to kube-system namespace
					To: []networkingv1.NetworkPolicyPeer{
						{
							NamespaceSelector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"name": "kube-system",
								},
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port: &dnsPort,
						},
						{
							Port: &dnsPort,
						},
					},
				},
			},
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, policy, n.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := n.client.Create(ctx, policy); err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Debug("DNS access policy already exists", zap.String("name", policyName))
			return nil
		}
		return fmt.Errorf("failed to create DNS access policy: %w", err)
	}

	n.logger.Info("Created DNS access network policy", zap.String("name", policyName))
	return nil
}

// applyCustomNetworkPolicies applies custom network policies from the tenant spec
func (n *NetworkManager) applyCustomNetworkPolicies(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string) error {
	networkSpec := managedRoost.Spec.Tenancy.NetworkPolicy
	if networkSpec == nil {
		return nil
	}

	// Apply custom ingress rules
	if len(networkSpec.Ingress) > 0 {
		if err := n.createCustomIngressPolicy(ctx, managedRoost, tenantID, networkSpec.Ingress); err != nil {
			return fmt.Errorf("failed to create custom ingress policy: %w", err)
		}
	}

	// Apply custom egress rules
	if len(networkSpec.Egress) > 0 {
		if err := n.createCustomEgressPolicy(ctx, managedRoost, tenantID, networkSpec.Egress); err != nil {
			return fmt.Errorf("failed to create custom egress policy: %w", err)
		}
	}

	return nil
}

// createCustomIngressPolicy creates a network policy with custom ingress rules
func (n *NetworkManager) createCustomIngressPolicy(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string, ingressRules []roostv1alpha1.NetworkPolicyRule) error {
	policyName := fmt.Sprintf("roost-keeper-tenant-%s-custom-ingress", tenantID)

	// Convert custom rules to Kubernetes NetworkPolicy format
	var k8sIngressRules []networkingv1.NetworkPolicyIngressRule
	for _, rule := range ingressRules {
		k8sRule := networkingv1.NetworkPolicyIngressRule{}

		// Convert 'From' peers
		for _, peer := range rule.From {
			k8sPeer := networkingv1.NetworkPolicyPeer{}
			if len(peer.PodSelector) > 0 {
				k8sPeer.PodSelector = &metav1.LabelSelector{
					MatchLabels: peer.PodSelector,
				}
			}
			if len(peer.NamespaceSelector) > 0 {
				k8sPeer.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: peer.NamespaceSelector,
				}
			}
			k8sRule.From = append(k8sRule.From, k8sPeer)
		}

		// Convert ports
		for _, port := range rule.Ports {
			k8sPort := networkingv1.NetworkPolicyPort{}
			if port.Port != nil {
				k8sPort.Port = port.Port
			}
			k8sRule.Ports = append(k8sRule.Ports, k8sPort)
		}

		k8sIngressRules = append(k8sIngressRules, k8sRule)
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentNetworkPolicy,
				"policy-type":        "custom-ingress",
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					TenantIDLabel: tenantID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: k8sIngressRules,
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, policy, n.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := n.client.Create(ctx, policy); err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Debug("Custom ingress policy already exists", zap.String("name", policyName))
			return nil
		}
		return fmt.Errorf("failed to create custom ingress policy: %w", err)
	}

	n.logger.Info("Created custom ingress network policy", zap.String("name", policyName))
	return nil
}

// createCustomEgressPolicy creates a network policy with custom egress rules
func (n *NetworkManager) createCustomEgressPolicy(ctx context.Context, managedRoost *roostv1alpha1.ManagedRoost, tenantID string, egressRules []roostv1alpha1.NetworkPolicyRule) error {
	policyName := fmt.Sprintf("roost-keeper-tenant-%s-custom-egress", tenantID)

	// Convert custom rules to Kubernetes NetworkPolicy format
	var k8sEgressRules []networkingv1.NetworkPolicyEgressRule
	for _, rule := range egressRules {
		k8sRule := networkingv1.NetworkPolicyEgressRule{}

		// Convert 'To' peers
		for _, peer := range rule.To {
			k8sPeer := networkingv1.NetworkPolicyPeer{}
			if len(peer.PodSelector) > 0 {
				k8sPeer.PodSelector = &metav1.LabelSelector{
					MatchLabels: peer.PodSelector,
				}
			}
			if len(peer.NamespaceSelector) > 0 {
				k8sPeer.NamespaceSelector = &metav1.LabelSelector{
					MatchLabels: peer.NamespaceSelector,
				}
			}
			k8sRule.To = append(k8sRule.To, k8sPeer)
		}

		// Convert ports
		for _, port := range rule.Ports {
			k8sPort := networkingv1.NetworkPolicyPort{}
			if port.Port != nil {
				k8sPort.Port = port.Port
			}
			k8sRule.Ports = append(k8sRule.Ports, k8sPort)
		}

		k8sEgressRules = append(k8sEgressRules, k8sRule)
	}

	policy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      policyName,
			Namespace: managedRoost.Namespace,
			Labels: map[string]string{
				TenantIDLabel:        tenantID,
				TenantComponentLabel: ComponentNetworkPolicy,
				"policy-type":        "custom-egress",
			},
			Annotations: map[string]string{
				TenantCreatedByAnnotation: "roost-keeper",
			},
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					TenantIDLabel: tenantID,
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeEgress,
			},
			Egress: k8sEgressRules,
		},
	}

	// Set owner reference for cleanup
	if err := controllerutil.SetControllerReference(managedRoost, policy, n.client.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	if err := n.client.Create(ctx, policy); err != nil {
		if errors.IsAlreadyExists(err) {
			n.logger.Debug("Custom egress policy already exists", zap.String("name", policyName))
			return nil
		}
		return fmt.Errorf("failed to create custom egress policy: %w", err)
	}

	n.logger.Info("Created custom egress network policy", zap.String("name", policyName))
	return nil
}

// ValidateNetworkPolicyConfiguration validates the network policy configuration
func (n *NetworkManager) ValidateNetworkPolicyConfiguration(networkSpec *roostv1alpha1.NetworkPolicySpec) error {
	if networkSpec == nil {
		return nil
	}

	// Validate ingress rules
	for i, rule := range networkSpec.Ingress {
		if err := n.validateNetworkPolicyRule(rule, fmt.Sprintf("ingress[%d]", i)); err != nil {
			return err
		}
	}

	// Validate egress rules
	for i, rule := range networkSpec.Egress {
		if err := n.validateNetworkPolicyRule(rule, fmt.Sprintf("egress[%d]", i)); err != nil {
			return err
		}
	}

	return nil
}

// validateNetworkPolicyRule validates a single network policy rule
func (n *NetworkManager) validateNetworkPolicyRule(rule roostv1alpha1.NetworkPolicyRule, ruleType string) error {
	// Validate peers (From/To)
	for i, peer := range rule.From {
		if len(peer.PodSelector) == 0 && len(peer.NamespaceSelector) == 0 {
			return fmt.Errorf("%s.from[%d]: at least one of podSelector or namespaceSelector must be specified", ruleType, i)
		}
	}

	for i, peer := range rule.To {
		if len(peer.PodSelector) == 0 && len(peer.NamespaceSelector) == 0 {
			return fmt.Errorf("%s.to[%d]: at least one of podSelector or namespaceSelector must be specified", ruleType, i)
		}
	}

	// Validate ports
	for i, port := range rule.Ports {
		if port.Protocol != "" {
			if port.Protocol != "TCP" && port.Protocol != "UDP" && port.Protocol != "SCTP" {
				return fmt.Errorf("%s.ports[%d]: protocol must be TCP, UDP, or SCTP", ruleType, i)
			}
		}
	}

	return nil
}
