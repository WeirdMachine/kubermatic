package test

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Masterminds/semver"
	"github.com/ghodss/yaml"
	"github.com/pmezard/go-difflib/difflib"

	apiv2 "github.com/kubermatic/kubermatic/api/pkg/api/v2"
	kubermaticv1 "github.com/kubermatic/kubermatic/api/pkg/crd/kubermatic/v1"
	"github.com/kubermatic/kubermatic/api/pkg/provider"
	"github.com/kubermatic/kubermatic/api/pkg/resources"
	"github.com/kubermatic/kubermatic/api/pkg/resources/apiserver"
	"github.com/kubermatic/kubermatic/api/pkg/resources/cloudconfig"
	"github.com/kubermatic/kubermatic/api/pkg/resources/controllermanager"
	machine2 "github.com/kubermatic/kubermatic/api/pkg/resources/machine"
	"github.com/kubermatic/kubermatic/api/pkg/resources/machinecontroler"
	"github.com/kubermatic/kubermatic/api/pkg/resources/openvpn"
	"github.com/kubermatic/kubermatic/api/pkg/resources/prometheus"
	"github.com/kubermatic/kubermatic/api/pkg/resources/scheduler"
	"github.com/kubermatic/kubermatic/api/pkg/version"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	kubefake "k8s.io/client-go/kubernetes/fake"
)

var update = flag.Bool("update", false, "Update test fixtures")

func checkTestResult(t *testing.T, resFile string, testObj interface{}) {
	path := filepath.Join("./fixtures", resFile+".yaml")
	jsonRes, err := json.Marshal(testObj)
	if err != nil {
		t.Fatal(err)
	}
	res, err := yaml.JSONToYAML(jsonRes)
	if err != nil {
		t.Fatal(err)
	}

	if *update {
		if err := ioutil.WriteFile(path, res, 0644); err != nil {
			t.Fatalf("failed to update fixtures: %v", err)
		}
	}

	exp, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	resStr := strings.TrimSpace(string(res))
	expStr := strings.TrimSpace(string(exp))

	diff := difflib.UnifiedDiff{
		A:        difflib.SplitLines(expStr),
		B:        difflib.SplitLines(resStr),
		FromFile: "Fixture",
		ToFile:   "Current",
		Context:  3,
	}
	diffStr, err := difflib.GetUnifiedDiffString(diff)
	if err != nil {
		t.Fatal(err)
	}

	if resStr != expStr {
		t.Errorf("\nDeployment file changed and does not match fixture(%q) anymore: \n %s\n\nMake sure you update all fixtures after changing templates.", path, diffStr)
	}
}

func TestLoadFiles(t *testing.T) {
	versions := []*version.MasterVersion{
		{
			Version: semver.MustParse("1.8.0"),
		},
		{
			Version: semver.MustParse("1.9.0"),
		},
	}

	clouds := map[string]*kubermaticv1.CloudSpec{
		"digitalocean": {
			Digitalocean: &kubermaticv1.DigitaloceanCloudSpec{
				Token: "do-token",
			},
		},
		"aws": {
			AWS: &kubermaticv1.AWSCloudSpec{
				AccessKeyID:         "aws-access-key-id",
				SecretAccessKey:     "aws-secret-access-key",
				AvailabilityZone:    "aws-availability-zone",
				InstanceProfileName: "aws-instance-profile-name",
				RoleName:            "aws-role-name",
				RouteTableID:        "aws-route-table-id",
				SecurityGroupID:     "aws-security-group",
				SubnetID:            "aws-subnet-id",
				VPCID:               "aws-vpn-id",
			},
		},
		"openstack": {
			Openstack: &kubermaticv1.OpenstackCloudSpec{
				SubnetID:       "openstack-subnet-id",
				Username:       "openstack-username",
				Tenant:         "openstack-tenant",
				Domain:         "openstack-domain",
				FloatingIPPool: "openstack-floating-ip-pool",
				Network:        "openstack-network",
				Password:       "openstack-password",
				RouterID:       "openstack-router-id",
				SecurityGroups: "openstack-security-group1,openstack-security-group2",
			},
		},
		"bringyourown": {
			BringYourOwn: &kubermaticv1.BringYourOwnCloudSpec{},
		},
	}

	dc := provider.DatacenterMeta{
		Spec: provider.DatacenterSpec{
			AWS: &provider.AWSSpec{
				AMI:           "ami-aujakj",
				Region:        "us-central1",
				ZoneCharacter: "a",
			},
			Digitalocean: &provider.DigitaloceanSpec{
				Region: "fra1",
			},
			Openstack: &provider.OpenstackSpec{
				AuthURL:          "https://example.com:8000/v3",
				AvailabilityZone: "zone1",
				DNSServers:       []string{"8.8.8.8", "8.8.4.4"},
				IgnoreVolumeAZ:   true,
				Region:           "cbk",
			},
		},
	}

	for _, ver := range versions {
		for prov, cloudspec := range clouds {
			t.Run(fmt.Sprintf("resources-%s-%s", prov, ver.Version.String()), func(t *testing.T) {
				cluster := &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "de-test-01",
						UID:  types.UID("1234567890"),
					},
					Spec: kubermaticv1.ClusterSpec{
						Cloud:   cloudspec,
						Version: ver.Version.String(),
						ClusterNetwork: kubermaticv1.ClusterNetworkingConfig{
							Services: kubermaticv1.NetworkRanges{
								CIDRBlocks: []string{"10.10.10.0/24"},
							},
							Pods: kubermaticv1.NetworkRanges{
								CIDRBlocks: []string{"172.25.0.0/16"},
							},
							DNSDomain: "cluster.local",
						},
					},
					Address: kubermaticv1.ClusterAddress{
						ExternalName: "jh8j81chn.europe-west3-c.dev.kubermatic.io",
						IP:           "35.198.93.90",
						AdminToken:   "6hzr76.u8txpkk4vhgmtgdp",
					},
					Status: kubermaticv1.ClusterStatus{
						NamespaceName: "cluster-de-test-01",
					},
				}

				kubeClient := kubefake.NewSimpleClientset(
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.TokensSecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.ServiceAccountKeySecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.CACertSecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.ApiserverTLSSecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.KubeletClientCertificatesSecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Secret{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.CAKeySecretName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.ConfigMap{
						ObjectMeta: metav1.ObjectMeta{
							ResourceVersion: "123456",
							Name:            resources.CloudConfigConfigMapName,
							Namespace:       cluster.Status.NamespaceName,
						},
					},
					&v1.Service{
						ObjectMeta: metav1.ObjectMeta{
							Name:      resources.ApiserverExternalServiceName,
							Namespace: cluster.Status.NamespaceName,
						},
						Spec: v1.ServiceSpec{
							Ports: []v1.ServicePort{
								{
									NodePort: 30000,
								},
							},
						},
					})

				var group wait.Group
				defer group.Wait()
				stopCh := make(chan struct{})
				defer func() {
					close(stopCh)
				}()

				kubeInformerFactory := informers.NewSharedInformerFactory(kubeClient, 10*time.Millisecond)
				data := resources.NewTemplateData(
					cluster,
					&dc,
					kubeInformerFactory.Core().V1().Secrets().Lister(),
					kubeInformerFactory.Core().V1().ConfigMaps().Lister(),
					kubeInformerFactory.Core().V1().Services().Lister(),
					"",
					"",
					resource.MustParse("5Gi"),
				)
				kubeInformerFactory.Start(wait.NeverStop)
				kubeInformerFactory.WaitForCacheSync(wait.NeverStop)

				deps := map[string]resources.DeploymentCreator{
					fmt.Sprintf("deployment-%s-%s-scheduler", prov, ver.Version.String()):          scheduler.Deployment,
					fmt.Sprintf("deployment-%s-%s-controller-manager", prov, ver.Version.String()): controllermanager.Deployment,
					fmt.Sprintf("deployment-%s-%s-apiserver", prov, ver.Version.String()):          apiserver.Deployment,
					fmt.Sprintf("deployment-%s-%s-machine-controller", prov, ver.Version.String()): machinecontroller.Deployment,
				}

				for fixture, create := range deps {
					res, err := create(data, nil)
					if err != nil {
						t.Fatalf("failed to create Deployment for %s: %v", fixture, err)
					}

					checkTestResult(t, fixture, res)
				}

				cmCreators := map[string]resources.ConfigMapCreator{
					fmt.Sprintf("configmap-%s-%s-cloud-config", prov, ver.Version.String()): cloudconfig.ConfigMap,
					fmt.Sprintf("configmap-%s-%s-openvpn", prov, ver.Version.String()):      openvpn.ConfigMap,
					fmt.Sprintf("configmap-%s-%s-prometheus", prov, ver.Version.String()):   prometheus.ConfigMap,
				}
				for fixture, create := range cmCreators {
					res, err := create(data, nil)
					if err != nil {
						t.Fatalf("failed to create ConfigMap for %s: %v", fixture, err)
					}

					checkTestResult(t, fixture, res)
				}

				serviceCreators := map[string]resources.ServiceCreator{
					fmt.Sprintf("service-%s-%s-apiserver", prov, ver.Version.String()):          apiserver.Service,
					fmt.Sprintf("service-%s-%s-apiserver-external", prov, ver.Version.String()): apiserver.ExternalService,
					fmt.Sprintf("service-%s-%s-openvpn", prov, ver.Version.String()):            openvpn.Service,
				}

				for fixture, create := range serviceCreators {
					res, err := create(data, nil)
					if err != nil {
						t.Fatalf("failed to create Service for %s: %v", fixture, err)
					}

					checkTestResult(t, fixture, res)
				}
			})
		}
	}
}

type Data struct {
	Cluster    *kubermaticv1.Cluster
	Node       *apiv2.Node
	Datacenter provider.DatacenterMeta
	Name       string
	Keys       []*kubermaticv1.UserSSHKey
}

func TestExecute(t *testing.T) {
	tests := map[string]struct {
		name string
		data Data
		ret  error
	}{
		"machine-digitalocean": {
			name: "get valid machine.yaml for Digitalocean",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "docluster-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "do-fra1",
							Digitalocean: &kubermaticv1.DigitaloceanCloudSpec{
								Token: "digitalocean-token",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "docluster-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							Digitalocean: &apiv2.DigitaloceanNodeSpec{
								Size:       "s-1vcpu-1gb",
								Backups:    false,
								IPv6:       false,
								Monitoring: true,
								Tags: []string{
									"digitalocean-example-tag-1",
									"digitalocean-example-tag-2",
								},
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							Ubuntu: &apiv2.UbuntuSpec{
								DistUpgradeOnBoot: false,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.9.6",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "Frankfurt",
					Seed:     "europe-west3-c",
					Country:  "DE",
					Spec: provider.DatacenterSpec{
						Digitalocean: &provider.DigitaloceanSpec{
							Region: "fra1",
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"docluster-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
		"machine-aws": {
			name: "get valid machine.yaml for AWS",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "awscluster-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "aws-eu-central-1a",
							AWS: &kubermaticv1.AWSCloudSpec{
								AccessKeyID:         "aws-access-key-id",
								SecretAccessKey:     "aws-secret-access-key",
								VPCID:               "aws-vpc-ic",
								SubnetID:            "aws-subnet-id",
								RoleName:            "aws-role-name",
								RouteTableID:        "aws-route-table-id",
								InstanceProfileName: "aws-instance-profile-name",
								SecurityGroupID:     "aws-security-group-id",
								AvailabilityZone:    "aws-availability-zone",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "awscluster-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							AWS: &apiv2.AWSNodeSpec{
								InstanceType: "t2.micro",
								VolumeSize:   25,
								VolumeType:   "standard",
								AMI:          "aws-ami",
								Tags: map[string]string{
									"AWSExampleTagKey1": "AWSExampleTagValue1",
									"AWSExampleTagKey2": "AWSExampleTagValue2",
								},
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							Ubuntu: &apiv2.UbuntuSpec{
								DistUpgradeOnBoot: false,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.9.6",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "Frankfurt",
					Seed:     "europe-west3-c",
					Country:  "DE",
					Spec: provider.DatacenterSpec{
						AWS: &provider.AWSSpec{
							Region:        "fra1",
							AMI:           "aws-ami",
							ZoneCharacter: "aws-zone-character",
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"awscluster-1a2b3c4d5e",
							},
						},
					},
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name-2",
							Fingerprint: "9876:54321:9876:54321",
							PublicKey:   "ssh-rsa TEST456test",
							Clusters: []string{
								"awscluster-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
		"machine-openstack": {
			name: "get valid machine.yaml for Openstack",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "openstackcluster-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "syseleven-dbl1",
							Openstack: &kubermaticv1.OpenstackCloudSpec{
								Username:       "os-username",
								Password:       "os-password",
								Tenant:         "os-tenant",
								Domain:         "os-domain",
								Network:        "os-network",
								SecurityGroups: "os-security-groups",
								FloatingIPPool: "os-floating-ip-pool",
								RouterID:       "os-router-id",
								SubnetID:       "os-subnet-id",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "openstackcluster-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							Openstack: &apiv2.OpenstackNodeSpec{
								Flavor: "os-flavor",
								Image:  "os-image",
								Tags: map[string]string{
									"foo": "bar",
								},
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							Ubuntu: &apiv2.UbuntuSpec{
								DistUpgradeOnBoot: false,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.9.6",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "Frankfurt",
					Seed:     "europe-west3-c",
					Country:  "DE",
					Spec: provider.DatacenterSpec{
						Openstack: &provider.OpenstackSpec{
							AuthURL:          "os-auth-url",
							AvailabilityZone: "os-availability-zone",
							Region:           "os-region",
							IgnoreVolumeAZ:   false,
							DNSServers:       []string{},
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"openstackcluster-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
		"machine-azure": {
			name: "get valid machine.yaml for Azure",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "azurecluster-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "whatever-dc",
							Azure: &kubermaticv1.AzureCloudSpec{
								TenantID:       "38w7giefb32fhifw3q",
								SubscriptionID: "32h9q8r8xqp3h9",
								ClientID:       "32hrf23oh89f32",
								ClientSecret:   "rbyughv438oh32f23v2",
								ResourceGroup:  "cluster-azurecluster-1a2b3c4d5e",
								VNetName:       "cluster-azurecluster-1a2b3c4d5e",
								SubnetName:     "cluster-azurecluster-1a2b3c4d5e",
								RouteTableName: "cluster-azurecluster-1a2b3c4d5e",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "azurecluster-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							Azure: &apiv2.AzureNodeSpec{
								Size:           "Standard_B1ms",
								AssignPublicIP: false,
								Tags: map[string]string{
									"foo": "bar",
								},
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							ContainerLinux: &apiv2.ContainerLinuxSpec{
								DisableAutoUpdate: true,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.10.3",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "westeurope",
					Seed:     "europe-west3-c",
					Country:  "NL",
					Spec: provider.DatacenterSpec{
						Azure: &provider.AzureSpec{
							Location: "westeurope",
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"azurecluster-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
		"machine-hetzner": {
			name: "get valid machine.yaml for Hetzner",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hetznercluster-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "hetzner-fsn1",
							Hetzner: &kubermaticv1.HetznerCloudSpec{
								Token: "hetzner-token",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "hetznercluster-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							Hetzner: &apiv2.HetznerNodeSpec{
								Type: "hetzner-type",
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							Ubuntu: &apiv2.UbuntuSpec{
								DistUpgradeOnBoot: false,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.9.6",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "Frankfurt",
					Seed:     "europe-west3-c",
					Country:  "DE",
					Spec: provider.DatacenterSpec{
						Hetzner: &provider.HetznerSpec{
							Datacenter: "hetzner-datacenter",
							Location:   "hetzner-location",
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"hetznercluster-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
		"machine-vsphere": {
			name: "get valid machine.yaml for VSphere",
			data: Data{
				Cluster: &kubermaticv1.Cluster{
					ObjectMeta: metav1.ObjectMeta{
						Name: "vsphere-1a2b3c4d5e",
					},
					Address: kubermaticv1.ClusterAddress{},
					Status:  kubermaticv1.ClusterStatus{},
					Spec: kubermaticv1.ClusterSpec{
						Cloud: &kubermaticv1.CloudSpec{
							DatacenterName: "vsphere-dummy",
							VSphere: &kubermaticv1.VSphereCloudSpec{
								Username: "vsphere-username",
								Password: "vsphere-password",
							},
						},
					},
				},
				Node: &apiv2.Node{
					Metadata: apiv2.ObjectMeta{
						Name: "vsphere-1a2b3c4d5e-te5s7",
					},
					Spec: apiv2.NodeSpec{
						Cloud: apiv2.NodeCloudSpec{
							VSphere: &apiv2.VSphereNodeSpec{
								Memory: 2048,
								CPUs:   2,
							},
						},
						OperatingSystem: apiv2.OperatingSystemSpec{
							Ubuntu: &apiv2.UbuntuSpec{
								DistUpgradeOnBoot: false,
							},
						},
						Versions: apiv2.NodeVersionInfo{
							Kubelet: "v1.9.6",
						},
					},
					Status: apiv2.NodeStatus{},
				},
				Datacenter: provider.DatacenterMeta{
					Location: "Frankfurt",
					Seed:     "europe-west3-c",
					Country:  "DE",
					Spec: provider.DatacenterSpec{
						VSphere: &provider.VSphereSpec{
							Cluster:       "vsphere-cluster",
							AllowInsecure: true,
							Datastore:     "vsphere-datastore",
							Endpoint:      "http://vsphere.local",
							Datacenter:    "vsphere-datacenter",
						},
					},
					IsSeed: false,
				},
				Keys: []*kubermaticv1.UserSSHKey{
					{
						Spec: kubermaticv1.SSHKeySpec{
							Owner:       "John Doe",
							Name:        "ssh-key-name",
							Fingerprint: "1234:56789:1234:56789",
							PublicKey:   "ssh-rsa TEST123test",
							Clusters: []string{
								"vsphere-1a2b3c4d5e",
							},
						},
					},
				},
			},
			ret: nil,
		},
	}

	for fixture, test := range tests {
		//TODO: Each test above needs to be executed for every supported version
		t.Run(test.name, func(t *testing.T) {
			machine, err := machine2.Machine(test.data.Cluster, test.data.Node, test.data.Datacenter, test.data.Keys)
			if err != nil {
				t.Fatalf("failed to generate machine: %v", err)
			}

			checkTestResult(t, fixture, machine)
		})
	}
}
