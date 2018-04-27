package main

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
)

func TestOverrideParentAnnotations(t *testing.T) {
	parentAnnotations := healthAnnotations{Port: "9080", EnableScrape: "false"}

	childAnnotations := healthAnnotations{Port: "8080", EnableScrape: "true"}
	overriddenAnnotations := overrideParentAnnotations(childAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations, childAnnotations)

	emptyPortAnnotations := healthAnnotations{Port: "", EnableScrape: "true"}
	overriddenAnnotations = overrideParentAnnotations(emptyPortAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations.Port, parentAnnotations.Port)
	assert.Equal(t, overriddenAnnotations.EnableScrape, emptyPortAnnotations.EnableScrape)

	emptyEnableScrapeAnnotations := healthAnnotations{Port: "8080", EnableScrape: ""}
	overriddenAnnotations = overrideParentAnnotations(emptyEnableScrapeAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations.Port, emptyEnableScrapeAnnotations.Port)
	assert.Equal(t, overriddenAnnotations.EnableScrape, parentAnnotations.EnableScrape)

	emptyAnnotations := healthAnnotations{Port: "", EnableScrape: ""}
	overriddenAnnotations = overrideParentAnnotations(emptyAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations, parentAnnotations)
}

func TestGetHealthAnnotations(t *testing.T) {
	client := &mockK8Client{}

	ns, err := client.Core().Namespaces().Get("energy", metav1.GetOptions{})
	if err != nil {
		assert.Fail(t, "Error getting namespace.")
	}

	retrievedAnnotations, err := getHealthAnnotations(*ns)
	if err != nil {
		assert.Fail(t, "Error getting annotations from namespace.")
	}

	assert.Equal(t, "true", retrievedAnnotations.EnableScrape)
	assert.Equal(t, "8080", retrievedAnnotations.Port)

	s, err := client.Core().Services("energy").Get("test-service", metav1.GetOptions{})
	if err != nil {
		assert.Fail(t, "Error getting namespace.")
	}

	retrievedAnnotations, err = getHealthAnnotations(*s)
	if err != nil {
		assert.Fail(t, "Error getting annotations from namespace.")
	}
	assert.Equal(t, "false", retrievedAnnotations.EnableScrape)
	assert.Equal(t, "8081", retrievedAnnotations.Port)
}

func TestGetClusterHealthcheckConfig(t *testing.T) {
	namespaces := make(chan namespace, 10)
	services := make(chan service, 10)
	errs := make(chan error, 10)

	s := &serviceDiscovery{client: &mockK8Client{}, label: "	", namespaces: namespaces, services: services, errors: errs}

	go func() {
		s.getClusterHealthcheckConfig()
		close(errs)
	}()

	select {
	case <-errs:
		t.Errorf("Should not get an error")

	case s := <-services:
		assert.Equal(t, "test-service", s.Name)
		assert.Equal(t, "energy", s.Namespace)
		assert.Equal(t, "http://test-service.energy:8081/__/health", s.HealthcheckURL)
		assert.Equal(t, "8081", s.HealthAnnotations.Port)
		assert.Equal(t, "false", s.HealthAnnotations.EnableScrape)

	case n := <-namespaces:
		assert.Equal(t, "energy", n.Name)
		assert.Equal(t, "8080", n.HealthAnnotations.Port)
		assert.Equal(t, "true", n.HealthAnnotations.EnableScrape)
	}
}

type mockK8Client struct {
}

func (m mockK8Client) Core() v1core.CoreV1Interface {
	return &mockCoreClient{}
}

type mockCoreClient struct {
}

func (c *mockCoreClient) Namespaces() v1core.NamespaceInterface {
	return &mockNamespaceClient{}
}

func (c *mockCoreClient) Services(namespace string) v1core.ServiceInterface {
	return &mockServiceClient{services: &v1.ServiceList{Items: []v1.Service{{ObjectMeta: metav1.ObjectMeta{Name: "test-service"}}}}}
}

func (c *mockCoreClient) RESTClient() rest.Interface {
	return nil
}

func (c *mockCoreClient) ComponentStatuses() v1core.ComponentStatusInterface {
	return nil
}

func (c *mockCoreClient) ConfigMaps(namespace string) v1core.ConfigMapInterface {
	return nil
}

func (c *mockCoreClient) Endpoints(namespace string) v1core.EndpointsInterface {
	return nil
}

func (c *mockCoreClient) Events(namespace string) v1core.EventInterface {
	return nil
}

func (c *mockCoreClient) LimitRanges(namespace string) v1core.LimitRangeInterface {
	return nil
}

func (c *mockCoreClient) Nodes() v1core.NodeInterface {
	return nil
}

func (c *mockCoreClient) PersistentVolumes() v1core.PersistentVolumeInterface {
	return nil
}

func (c *mockCoreClient) PersistentVolumeClaims(namespace string) v1core.PersistentVolumeClaimInterface {
	return nil
}

func (c *mockCoreClient) Pods(namespace string) v1core.PodInterface {
	return nil
}

func (c *mockCoreClient) PodTemplates(namespace string) v1core.PodTemplateInterface {
	return nil
}

func (c *mockCoreClient) ReplicationControllers(namespace string) v1core.ReplicationControllerInterface {
	return nil
}

func (c *mockCoreClient) ResourceQuotas(namespace string) v1core.ResourceQuotaInterface {
	return nil
}

func (c *mockCoreClient) Secrets(namespace string) v1core.SecretInterface {
	return nil
}

func (c *mockCoreClient) ServiceAccounts(namespace string) v1core.ServiceAccountInterface {
	return nil
}

type mockNamespaceClient struct {
}

func (n *mockNamespaceClient) List(opts metav1.ListOptions) (*v1.NamespaceList, error) {
	ns, err := n.Get("energy", metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("failed getting namespace")
	}
	return &v1.NamespaceList{Items: []v1.Namespace{*ns}}, nil
}

func (n *mockNamespaceClient) Create(*v1.Namespace) (*v1.Namespace, error) {
	return &v1.Namespace{}, nil
}
func (n *mockNamespaceClient) Update(*v1.Namespace) (*v1.Namespace, error) {
	return &v1.Namespace{}, nil
}
func (n *mockNamespaceClient) UpdateStatus(*v1.Namespace) (*v1.Namespace, error) {
	return &v1.Namespace{}, nil
}
func (n *mockNamespaceClient) Delete(name string, options *metav1.DeleteOptions) error {
	return nil
}
func (n *mockNamespaceClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}
func (n *mockNamespaceClient) Get(name string, options metav1.GetOptions) (*v1.Namespace, error) {
	annotations := make(map[string]string)
	annotations["uw.health.aggregator.port"] = "8080"
	annotations["uw.health.aggregator.enable"] = "true"
	return &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "energy", Annotations: annotations}}, nil
}
func (n *mockNamespaceClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}
func (n *mockNamespaceClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Namespace, err error) {
	return &v1.Namespace{}, nil
}

func (n *mockNamespaceClient) Finalize(item *v1.Namespace) (*v1.Namespace, error) {
	return &v1.Namespace{}, nil
}

type mockServiceClient struct {
	services *v1.ServiceList
}

func (c *mockServiceClient) List(opts metav1.ListOptions) (*v1.ServiceList, error) {
	s, err := c.Get("test-service", metav1.GetOptions{})
	if err != nil {
		return nil, errors.New("failed getting service")
	}
	return &v1.ServiceList{Items: []v1.Service{*s}}, nil
}

func (c *mockServiceClient) Create(service *v1.Service) (result *v1.Service, err error) {
	return &v1.Service{}, nil
}

func (c *mockServiceClient) Update(service *v1.Service) (result *v1.Service, err error) {
	return &v1.Service{}, nil
}

func (c *mockServiceClient) UpdateStatus(service *v1.Service) (result *v1.Service, err error) {
	return &v1.Service{}, nil
}

func (c *mockServiceClient) Delete(name string, options *metav1.DeleteOptions) error {
	return nil
}

func (c *mockServiceClient) DeleteCollection(options *metav1.DeleteOptions, listOptions metav1.ListOptions) error {
	return nil
}

func (c *mockServiceClient) Get(name string, options metav1.GetOptions) (result *v1.Service, err error) {
	annotations := make(map[string]string)
	annotations["uw.health.aggregator.port"] = "8081"
	annotations["uw.health.aggregator.enable"] = "false"
	return &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Annotations: annotations}}, nil
}

func (c *mockServiceClient) Watch(opts metav1.ListOptions) (watch.Interface, error) {
	return nil, nil
}

func (c *mockServiceClient) Patch(name string, pt types.PatchType, data []byte, subresources ...string) (result *v1.Service, err error) {
	return &v1.Service{}, nil
}

func (c *mockServiceClient) ProxyGet(scheme, name, port, path string, params map[string]string) rest.ResponseWrapper {
	return nil
}
