package discovery

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/stretchr/testify/assert"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func Test_OverrideParentAnnotations(t *testing.T) {
	parentAnnotations := model.HealthAnnotations{Port: "9080", EnableScrape: "false"}

	childAnnotations := model.HealthAnnotations{Port: "8080", EnableScrape: "true"}
	overriddenAnnotations := overrideParentAnnotations(childAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations, childAnnotations)

	emptyPortAnnotations := model.HealthAnnotations{Port: "", EnableScrape: "true"}
	overriddenAnnotations = overrideParentAnnotations(emptyPortAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations.Port, parentAnnotations.Port)
	assert.Equal(t, overriddenAnnotations.EnableScrape, emptyPortAnnotations.EnableScrape)

	emptyEnableScrapeAnnotations := model.HealthAnnotations{Port: "8080", EnableScrape: ""}
	overriddenAnnotations = overrideParentAnnotations(emptyEnableScrapeAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations.Port, emptyEnableScrapeAnnotations.Port)
	assert.Equal(t, overriddenAnnotations.EnableScrape, parentAnnotations.EnableScrape)

	emptyAnnotations := model.HealthAnnotations{Port: "", EnableScrape: ""}
	overriddenAnnotations = overrideParentAnnotations(emptyAnnotations, parentAnnotations)

	assert.Equal(t, overriddenAnnotations, parentAnnotations)
}
func Test_GetHealthAnnotations(t *testing.T) {
	client := setUpTest(t)

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
		assert.Fail(t, "Error getting service.")
	}

	retrievedAnnotations, err = getHealthAnnotations(*s)
	if err != nil {
		assert.Fail(t, "Error getting annotations from namespace.")
	}
	assert.Equal(t, "false", retrievedAnnotations.EnableScrape)
	assert.Equal(t, "8081", retrievedAnnotations.Port)
}
func Test_GetClusterHealthcheckConfig(t *testing.T) {
	client := setUpTest(t)

	namespaces := make(chan model.Namespace, 10)
	services := make(chan model.Service, 10)
	errs := make(chan error, 10)

	s := &KubeDiscoveryService{K8sClient: client, Namespaces: namespaces, Services: services, Errors: errs}

	go func() {
		s.GetClusterHealthcheckConfig()
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

func setUpTest(t *testing.T) *fake.Clientset {

	annotations := make(map[string]string)
	annotations["uw.health.aggregator.port"] = "8080"
	annotations["uw.health.aggregator.enable"] = "true"
	testNamespace := &v1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: "energy", Annotations: annotations}}

	svcAnnotations := make(map[string]string)
	svcAnnotations["uw.health.aggregator.port"] = "8081"
	svcAnnotations["uw.health.aggregator.enable"] = "false"
	testService := &v1.Service{ObjectMeta: metav1.ObjectMeta{Name: "test-service", Namespace: "energy", Annotations: svcAnnotations}}

	client := fake.NewSimpleClientset()

	_, nsErr := client.Core().Namespaces().Create(testNamespace)
	require.NoError(t, nsErr)

	_, sErr := client.Core().Services(testNamespace.Name).Create(testService)
	require.NoError(t, sErr)

	return client
}
