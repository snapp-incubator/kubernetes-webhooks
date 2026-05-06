package v1

import (
	"context"
	"fmt"
	"net"
	"net/netip"
	"slices"
	"strings"

	ciliumv2alpha1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// nolint:unused
// log is for logging in this package.
var servicelog = logf.Log.WithName("service-resource")

const (
	addressPoolLabel         = "network.snappcloud.io/address-pool"
	defaultAddressPool       = "default"
	lbIpamIpsAnnotation      = "io.cilium/lb-ipam-ips"
	lbIpamIpsAnnotationAlias = "lbipam.cilium.io/ips"
)

// SetupServiceWebhookWithManager registers the webhook for Service in the manager.
func SetupServiceWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&corev1.Service{}).
		WithValidator(&ServiceCustomValidator{
			client: mgr.GetClient(),
		}).
		Complete()
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: The 'path' attribute must follow a specific pattern and should not be modified directly here.
// Modifying the path for an invalid path can cause API server errors; failing to locate the webhook.
// +kubebuilder:webhook:path=/validate--v1-service,mutating=false,failurePolicy=fail,sideEffects=None,groups="",resources=services,verbs=create;update,versions=v1,name=vservice-v1.spcld.io,admissionReviewVersions=v1

// ServiceCustomValidator struct is responsible for validating the Service resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type ServiceCustomValidator struct {
	client client.Client
}

var _ webhook.CustomValidator = &ServiceCustomValidator{}

func (v *ServiceCustomValidator) ValidateCreate(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	service, ok := obj.(*corev1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object but got %T", obj)
	}

	warnings := admission.Warnings{}

	w, err := v.validateIPSources(ctx, service)
	warnings = append(warnings, w...)
	if err != nil {
		return warnings, err
	}

	return warnings, nil
}

func (v *ServiceCustomValidator) ValidateUpdate(ctx context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	service, ok := newObj.(*corev1.Service)
	if !ok {
		return nil, fmt.Errorf("expected a Service object for the newObj but got %T", newObj)
	}

	warnings := admission.Warnings{}

	w, err := v.validateIPSources(ctx, service)
	warnings = append(warnings, w...)
	if err != nil {
		return warnings, err
	}

	return warnings, nil
}

func (v *ServiceCustomValidator) ValidateDelete(ctx context.Context, obj runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

// validateIPSources ensures at most one of the two IPAM annotations or
// spec.loadBalancerIP is set, and that whichever IP(s) are specified fall
// within the CiliumLoadBalancerIPPool named by the address-pool label.
// The function is a no-op when none of the three sources is set, or when
// the address-pool label is absent.
func (v *ServiceCustomValidator) validateIPSources(ctx context.Context, service *corev1.Service) (admission.Warnings, error) {
	annotations := service.GetAnnotations()
	if annotations == nil {
		annotations = map[string]string{}
	}
	sources := []ipSourceRequest{
		{name: lbIpamIpsAnnotation, ips: parseRequestedIPs(annotations[lbIpamIpsAnnotation])},
		{name: lbIpamIpsAnnotationAlias, ips: parseRequestedIPs(annotations[lbIpamIpsAnnotationAlias])},
		{name: "spec.loadBalancerIP", ips: parseRequestedIPs(service.Spec.LoadBalancerIP)},
	}

	var configuredSources []ipSourceRequest
	for _, source := range sources {
		if len(source.ips) > 0 {
			configuredSources = append(configuredSources, source)
		}
	}

	if len(configuredSources) == 0 {
		return nil, nil
	}
	if err := validateMatchingIPSources(configuredSources); err != nil {
		return nil, err
	}

	cidrs, ranges, poolName, err := v.fetchPoolBlocks(ctx, service)
	if err != nil {
		return nil, err
	}

	for _, ip := range configuredSources[0].ips {
		if !isIPInPool(ip, cidrs, ranges) {
			return nil, fmt.Errorf(
				"IP %s is not within any block of CiliumLoadBalancerIPPool %q",
				ip, poolName,
			)
		}
	}

	return nil, nil
}

func validateMatchingIPSources(sources []ipSourceRequest) error {
	if len(sources) < 2 {
		return nil
	}

	baseline := sources[0]
	for _, source := range sources[1:] {
		if !equalRequestedIPs(baseline.ips, source.ips) {
			return fmt.Errorf(
				"when multiple IP sources are set, %s and %s must specify the same IP value(s)",
				baseline.name,
				source.name,
			)
		}
	}

	return nil
}

func parseRequestedIPs(raw string) []string {
	var ips []string
	for _, part := range strings.Split(raw, ",") {
		if ip := strings.TrimSpace(part); ip != "" {
			ips = append(ips, ip)
		}
	}

	return ips
}

func equalRequestedIPs(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}

	// Create new slices for sorting to keep order of original slices
	leftSorted := append([]string(nil), left...)
	rightSorted := append([]string(nil), right...)
	slices.Sort(leftSorted)
	slices.Sort(rightSorted)

	return slices.Equal(leftSorted, rightSorted)
}

// fetchPoolBlocks retrieves the CiliumLoadBalancerIPPool named by the service's
// `network.snappcloud.io/address-pool` label and parses its blocks.
// When the label is absent the pool name defaults to "default".
func (v *ServiceCustomValidator) fetchPoolBlocks(ctx context.Context, service *corev1.Service) ([]*net.IPNet, []ipRange, string, error) {
	poolName := service.GetLabels()[addressPoolLabel]
	if poolName == "" {
		poolName = defaultAddressPool
	}

	ipPool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{}
	if err := v.client.Get(ctx, types.NamespacedName{Name: poolName}, ipPool); err != nil {
		return nil, nil, poolName, fmt.Errorf("failed to get CiliumLoadBalancerIPPool %q: %w", poolName, err)
	}

	cidrs, ranges, err := parseIPPoolBlocks(ipPool, poolName)
	if err != nil {
		return nil, nil, poolName, err
	}

	return cidrs, ranges, poolName, nil
}

// parseIPPoolBlocks parses the CIDR and start/stop blocks from a CiliumLoadBalancerIPPool.
func parseIPPoolBlocks(ipPool *ciliumv2alpha1.CiliumLoadBalancerIPPool, poolName string) ([]*net.IPNet, []ipRange, error) {
	var cidrs []*net.IPNet
	var ranges []ipRange

	for _, block := range ipPool.Spec.Blocks {
		if cidrStr := string(block.Cidr); cidrStr != "" {
			_, ipNet, err := net.ParseCIDR(cidrStr)
			if err != nil {
				return nil, nil, fmt.Errorf("invalid CIDR %q in CiliumLoadBalancerIPPool %q: %w", cidrStr, poolName, err)
			}
			cidrs = append(cidrs, ipNet)
		}

		hasStart := block.Start != ""
		hasStop := block.Stop != ""
		if hasStart != hasStop {
			return nil, nil, fmt.Errorf(
				"invalid block in CiliumLoadBalancerIPPool %q: both start and stop must be set together", poolName,
			)
		}
		if hasStart {
			startAddr, err := netip.ParseAddr(block.Start)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse start ip %q in CiliumLoadBalancerIPPool %q: %w", block.Start, poolName, err)
			}
			endAddr, err := netip.ParseAddr(block.Stop)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to parse end ip %q in CiliumLoadBalancerIPPool %q: %w", block.Stop, poolName, err)
			}
			if startAddr.Compare(endAddr) > 0 {
				return nil, nil, fmt.Errorf(
					"invalid start/stop range in CiliumLoadBalancerIPPool %q: start %q is greater than stop %q",
					poolName, block.Start, block.Stop,
				)
			}
			ranges = append(ranges, ipRange{start: startAddr, end: endAddr})
		}
	}

	return cidrs, ranges, nil
}

// isIPInPool reports whether the given IP string falls within any of the provided CIDRs or ranges.
func isIPInPool(ipStr string, cidrs []*net.IPNet, ranges []ipRange) bool {
	addr, err := netip.ParseAddr(ipStr)
	if err != nil {
		return false
	}
	for _, cidr := range cidrs {
		if cidr.Contains(addr.AsSlice()) {
			return true
		}
	}
	for _, r := range ranges {
		if r.start.Compare(addr) <= 0 && r.end.Compare(addr) >= 0 {
			return true
		}
	}
	return false
}

// ipRange holds a parsed start/stop address range.
type ipRange struct {
	start, end netip.Addr
}

type ipSourceRequest struct {
	name string
	ips  []string
}
