package main

import (
	"fmt"

	log "github.com/sirupsen/logrus"
	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	v1core "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type serviceDiscovery struct {
	client       kubernetesClient
	label        string
	namespaces   chan namespace
	services     chan service
	healthchecks chan service
	errors       chan error
}

type kubeClient struct {
	client kubernetesClient
}

type kubernetesClient interface {
	Core() v1core.CoreV1Interface
}

func newKubeClient(kubeConfigPath string) *kubeClient {

	var config *rest.Config
	var err error
	if kubeConfigPath != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeConfigPath)
		outOfCluster = true
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

	return &kubeClient{client: kubeClientSet}
}

func (s *serviceDiscovery) getClusterHealthcheckConfig() {

	log.Info("loading namespace and service annotations")
	defaultAnnotations := healthAnnotations{EnableScrape: defaultEnableScrape, Port: defaultPort}

	namespaces, err := s.client.Core().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		select {
		case s.errors <- fmt.Errorf("Could not get namespaces via kubernetes api 1: (%v)", err):
		default:
		}
		return
	}

	for _, n := range namespaces.Items {
		namespaceAnnotations, err := getHealthAnnotations(n)
		if err != nil {
			select {
			case s.errors <- fmt.Errorf("Could not get namespace annotations via kubernetes api 2: (%v)", err):
			default:
			}
			return
		}

		namespaceAnnotations = overrideParentAnnotations(namespaceAnnotations, defaultAnnotations)

		s.namespaces <- namespace{
			Name:              n.Name,
			HealthAnnotations: namespaceAnnotations,
		}

		log.Debugf("Added namespace %v to channel\n", n.Name)

		services, err := s.client.Core().Services(n.Name).List(metav1.ListOptions{LabelSelector: s.label})
		if err != nil {
			select {
			case s.errors <- fmt.Errorf("Could not get services via kubernetes api: (%v)", err):
			default:
			}
			return
		}

		for _, svc := range services.Items {
			serviceAnnotations, err := getHealthAnnotations(svc)
			if err != nil {
				select {
				case s.errors <- fmt.Errorf("Could not get service annotations via kubernetes api: (%v)", err):
				default:
				}
				continue
			}
			serviceAnnotations = overrideParentAnnotations(serviceAnnotations, namespaceAnnotations)
			s.services <- service{
				Name:              svc.Name,
				Namespace:         n.Name,
				HealthcheckURL:    fmt.Sprintf("http://%s.%s:%s/__/health", svc.Name, n.Name, serviceAnnotations.Port),
				HealthAnnotations: serviceAnnotations,
			}
			log.Debugf("Added service %v to channel\n", svc.Name)
		}
	}
	close(s.services)
	close(s.namespaces)
}

func getHealthAnnotations(k8sObject interface{}) (healthAnnotations, error) {

	switch k8sObject.(type) {
	case v1.Namespace:
		ns := k8sObject.(v1.Namespace)
		annotations := ns.Annotations
		var h healthAnnotations
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
		var h healthAnnotations
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
		return healthAnnotations{}, err
	}
}

func overrideParentAnnotations(h healthAnnotations, overrides healthAnnotations) healthAnnotations {
	if h.Port == "" {
		h.Port = overrides.Port
	}
	if h.EnableScrape == "" {
		h.EnableScrape = overrides.EnableScrape
	}
	return h
}
