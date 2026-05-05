package v1

import (
	"context"
	"fmt"
	"net"
	"net/netip"
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
	ipamOriginalRaw := annotations[lbIpamIpsAnnotation]
	ipamAliasRaw := annotations[lbIpamIpsAnnotationAlias]
	loadBalancerIP := strings.TrimSpace(service.Spec.LoadBalancerIP)

	// Count how many sources are set.
	setCount := 0
	if ipamOriginalRaw != "" {
		setCount++
	}
	if ipamAliasRaw != "" {
		setCount++
	}
	if loadBalancerIP != "" {
		setCount++
	}

	if setCount > 1 {
		return nil, fmt.Errorf(
			"at most one of %s, %s, and spec.loadBalancerIP may be set",
			lbIpamIpsAnnotation, lbIpamIpsAnnotationAlias,
		)
	}
	if setCount == 0 {
		return nil, nil
	}

	cidrs, ranges, poolName, err := v.fetchPoolBlocks(ctx, service)
	if err != nil {
		return nil, err
	}

	// Collect the IPs to validate.
	var ipsToCheck []string
	if loadBalancerIP != "" {
		ipsToCheck = []string{loadBalancerIP}
	} else {
		raw := ipamOriginalRaw
		if raw == "" {
			raw = ipamAliasRaw
		}
		for _, s := range strings.Split(raw, ",") {
			if t := strings.TrimSpace(s); t != "" {
				ipsToCheck = append(ipsToCheck, t)
			}
		}
	}

	for _, ip := range ipsToCheck {
		if !isIPInPool(ip, cidrs, ranges) {
			return nil, fmt.Errorf(
				"IP %s is not within any block of CiliumLoadBalancerIPPool %q",
				ip, poolName,
			)
		}
	}

	return nil, nil
}

// fetchPoolBlocks retrieves the CiliumLoadBalancerIPPool named by the service's
// `network.snappcloud.io/address-pool` label and parses its blocks.
// When the label is absent the pool name defaults to "default".
func (v *ServiceCustomValidator) fetchPoolBlocks(ctx context.Context, service *corev1.Service) ([]*net.IPNet, []ipRange, string, error) {
	poolName := service.GetLabels()[addressPoolLabel]
	if poolName == "" {
		poolName = defaultAddressPool
	} else {
		poolName = poolName + "-pool"
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
