package discovery

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// ServiceDiscovery is a struct which contains fields required for the discovery of k8s Namespaces, Services
type ServiceDiscovery struct {
	K8sClient  kubernetes.Interface
	Label      string
	Namespaces chan model.Namespace
	Services   chan model.Service
	Errors     chan error
}

// NewKubeClient returns a KubeClient for in cluster or out of cluster operation depending on whether or
// not a kubeconfig file path is provided
func NewKubeClient(kubeConfigPath string) *kubernetes.Clientset {

	var config *rest.Config
	var err error
	if kubeConfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
	} else {
		config, err = rest.InClusterConfig()
	}
	if err != nil {
		log.Fatalf("Failed to create kubernetes client: %v", err)
	}

	kubeClientSet, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Panic(err)
	}

	return kubeClientSet
}

// GetClusterHealthcheckConfig method retrieves Namespace and Service annotations specific to health aggregator
func (s *ServiceDiscovery) GetClusterHealthcheckConfig() {

	log.Info("loading namespace and service annotations")
	defaultAnnotations := model.HealthAnnotations{EnableScrape: constants.DefaultEnableScrape, Port: constants.DefaultPort}

	namespaces, err := s.K8sClient.Core().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		select {
		case s.Errors <- fmt.Errorf("Could not get namespaces via kubernetes api 1: (%v)", err):
		default:
		}
		return
	}

	for _, n := range namespaces.Items {
		namespaceAnnotations, err := getHealthAnnotations(n)
		if err != nil {
			select {
			case s.Errors <- fmt.Errorf("Could not get namespace annotations via kubernetes api 2: (%v)", err):
			default:
			}
			return
		}

		namespaceAnnotations = overrideParentAnnotations(namespaceAnnotations, defaultAnnotations)

		s.Namespaces <- model.Namespace{
			Name:              n.Name,
			HealthAnnotations: namespaceAnnotations,
		}

		log.Debugf("Added namespace %v to channel\n", n.Name)

		services, err := s.K8sClient.Core().Services(n.Name).List(metav1.ListOptions{LabelSelector: s.Label})
		if err != nil {
			select {
			case s.Errors <- fmt.Errorf("Could not get services via kubernetes api: (%v)", err):
			default:
			}
			return
		}

		for _, svc := range services.Items {

			//deployment := s.K8sClient.Core().PodTemplates().Get()(metav1.ListOptions{})

			serviceAnnotations, err := getHealthAnnotations(svc)
			if err != nil {
				select {
				case s.Errors <- fmt.Errorf("Could not get service annotations via kubernetes api: (%v)", err):
				default:
				}
				continue
			}
			serviceAnnotations = overrideParentAnnotations(serviceAnnotations, namespaceAnnotations)
			s.Services <- model.Service{
				Name:              svc.Name,
				Namespace:         n.Name,
				HealthcheckURL:    fmt.Sprintf("http://%s.%s:%s/__/health", svc.Name, n.Name, serviceAnnotations.Port),
				HealthAnnotations: serviceAnnotations,
			}
			log.Debugf("Added service %v to channel\n", svc.Name)
		}
	}
}

// func (s *ServiceDiscovery) getDeployments(namespaceName string) (map[string]deployment, error) {
// 	deploymentList, err := s.Client.Core()  hs.k8sClient.ExtensionsV1beta1().Deployments().List(v1.ListOptions{})
// 	if err != nil {
// 		return nil, fmt.Errorf("failed to retrieve deployments: %v", err.Error())
// 	}

// 	deployments := make(map[string]deployment)
// 	for _, d := range deploymentList.Items {
// 		deployments[d.Name] = deployment{
// 			desiredReplicas: *d.Spec.Replicas,
// 		}
// 	}
// 	return deployments, nil
// }

func getHealthAnnotations(k8sObject interface{}) (model.HealthAnnotations, error) {

	switch k8sObject.(type) {
	case v1.Namespace:
		ns := k8sObject.(v1.Namespace)
		annotations := ns.Annotations
		var h model.HealthAnnotations
		for k, v := range annotations {
			if k == "uw.health.aggregator.port" {
				h.Port = v
			}
			if k == "uw.health.aggregator.enable" {
				if v == "true" || v == "false" {
					h.EnableScrape = v
				}
			}
		}
		return h, nil
	case v1.Service:
		svc := k8sObject.(v1.Service)
		annotations := svc.Annotations
		var h model.HealthAnnotations
		for k, v := range annotations {
			if k == "uw.health.aggregator.port" {
				h.Port = v
			}
			if k == "uw.health.aggregator.enable" {
				if v == "true" || v == "false" {
					h.EnableScrape = v
				}
			}
		}
		return h, nil
	default:
		err := fmt.Errorf("no health aggregator annotations found - passed type %T unknown", k8sObject)
		return model.HealthAnnotations{}, err
	}
}

func overrideParentAnnotations(h model.HealthAnnotations, overrides model.HealthAnnotations) model.HealthAnnotations {
	if h.Port == "" {
		h.Port = overrides.Port
	}
	if h.EnableScrape == "" {
		h.EnableScrape = overrides.EnableScrape
	}
	return h
}
