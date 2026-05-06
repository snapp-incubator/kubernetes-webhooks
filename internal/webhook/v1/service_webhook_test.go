package v1

import (
	ciliumv2alpha1 "github.com/cilium/cilium/pkg/k8s/apis/cilium.io/v2alpha1"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Service Webhook", func() {
	Context("When validating ipam IPs", func() {
		Context("When address-pool label is not set", func() {
			var (
				scheme    *runtime.Scheme
				validator ServiceCustomValidator
			)

			BeforeEach(func() {
				pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
						Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
							{Cidr: "10.0.0.0/24"},
						},
					},
				}
				scheme = runtime.NewScheme()
				Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())
				validator = ServiceCustomValidator{
					client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
				}
			})

			It("Should allow creation when the IP is inside the default pool CIDR", func() {
				obj := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Annotations: map[string]string{
							"io.cilium/lb-ipam-ips": "10.0.0.1",
						},
					},
				}
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should reject creation when the IP is outside the default pool CIDR", func() {
				obj := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
						Annotations: map[string]string{
							"io.cilium/lb-ipam-ips": "192.168.1.1",
						},
					},
				}
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("192.168.1.1"))
			})
		})

		It("Should allow creation when no ip source is set", func() {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my-pool",
					},
				},
			}
			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when CiliumLoadBalancerIPPool does not exist", func() {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "nonexistent-pool",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.1",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("failed to get CiliumLoadBalancerIPPool"))
		})

		It("Should allow creation when a single ipam IP exists in the pool CIDR", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should allow creation when two ipam IPs both exist in the pool CIDR", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5,10.0.0.10",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when one ipam IP is outside the pool CIDR", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5,192.168.1.1",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("192.168.1.1"))
		})

		It("Should allow creation when a single ipam IP exists in the pool CIDR using alias annotation", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"lbipam.cilium.io/ips": "10.0.0.5",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should allow creation when two ipam IPs both exist in the pool CIDR using alias annotation", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"lbipam.cilium.io/ips": "10.0.0.5,10.0.0.10",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when one ipam IP is outside the pool CIDR using alias annotation", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"lbipam.cilium.io/ips": "10.0.0.5,192.168.1.1",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("192.168.1.1"))
		})
		It("Should allow creation when both ipam annotations are set to the same value", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5,10.0.0.10",
						"lbipam.cilium.io/ips":  "10.0.0.10,10.0.0.5",
					},
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when the two ipam annotations are set to different values", func() {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.4,192.168.1.1",
						"lbipam.cilium.io/ips":  "10.0.0.4,192.168.1.2",
					},
				},
			}
			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(lbIpamIpsAnnotation))
			Expect(err.Error()).To(ContainSubstring(lbIpamIpsAnnotationAlias))
		})

		It("Should allow creation when lbIpamIps annotation and loadBalancerIP are set to the same value", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5,10.0.0.3",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "10.0.0.3,10.0.0.5",
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when lbIpamIps annotation and loadBalancerIP are set to different values", func() {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"io.cilium/lb-ipam-ips": "10.0.0.5,192.168.1.2",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "10.0.0.6,192.168.1.2",
				},
			}
			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(lbIpamIpsAnnotation))
			Expect(err.Error()).To(ContainSubstring("spec.loadBalancerIP"))
		})

		It("Should allow update when lbIpamIps alias annotation and loadBalancerIP are set to the same value", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"lbipam.cilium.io/ips": "10.0.0.5",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "10.0.0.5",
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateUpdate(ctx, nil, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when lbIpamIps alias annotation and loadBalancerIP are set to different values", func() {
			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
					Annotations: map[string]string{
						"lbipam.cilium.io/ips": "10.0.0.5",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "10.0.0.6",
				},
			}
			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring(lbIpamIpsAnnotationAlias))
			Expect(err.Error()).To(ContainSubstring("spec.loadBalancerIP"))
		})
	})

	Context("When validating spec.loadBalancerIP", func() {
		Context("When address-pool label is not set", func() {
			var (
				scheme    *runtime.Scheme
				validator ServiceCustomValidator
			)

			BeforeEach(func() {
				pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
					ObjectMeta: metav1.ObjectMeta{
						Name: "default",
					},
					Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
						Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
							{Cidr: "10.0.0.0/24"},
						},
					},
				}
				scheme = runtime.NewScheme()
				Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())
				validator = ServiceCustomValidator{
					client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
				}
			})

			It("Should allow creation when loadBalancerIP is inside the default pool CIDR", func() {
				obj := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						LoadBalancerIP: "10.0.0.5",
					},
				}
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).NotTo(HaveOccurred())
			})

			It("Should reject creation when loadBalancerIP is outside the default pool CIDR", func() {
				obj := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-svc",
						Namespace: "default",
					},
					Spec: corev1.ServiceSpec{
						LoadBalancerIP: "192.168.1.1",
					},
				}
				_, err := validator.ValidateCreate(ctx, obj)
				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("192.168.1.1"))
			})
		})

		It("Should allow creation when loadBalancerIP is inside the pool CIDR", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "10.0.0.5",
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).NotTo(HaveOccurred())
		})

		It("Should reject creation when loadBalancerIP is outside the pool CIDR", func() {
			pool := &ciliumv2alpha1.CiliumLoadBalancerIPPool{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-pool",
				},
				Spec: ciliumv2alpha1.CiliumLoadBalancerIPPoolSpec{
					Blocks: []ciliumv2alpha1.CiliumLoadBalancerIPPoolIPBlock{
						{Cidr: "10.0.0.0/24"},
					},
				},
			}

			obj := &corev1.Service{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-svc",
					Namespace: "default",
					Labels: map[string]string{
						"network.snappcloud.io/address-pool": "my",
					},
				},
				Spec: corev1.ServiceSpec{
					LoadBalancerIP: "192.168.1.1",
				},
			}

			scheme := runtime.NewScheme()
			Expect(ciliumv2alpha1.AddToScheme(scheme)).To(Succeed())

			validator := ServiceCustomValidator{
				client: fake.NewClientBuilder().WithScheme(scheme).WithObjects(pool).Build(),
			}
			_, err := validator.ValidateCreate(ctx, obj)
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("192.168.1.1"))
		})
	})
})
