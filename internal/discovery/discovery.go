package discovery

import (
	"fmt"
	"strconv"

	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"github.com/utilitywarehouse/health-aggregator/internal/constants"
	"github.com/utilitywarehouse/health-aggregator/internal/db"
	"github.com/utilitywarehouse/health-aggregator/internal/model"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	watch "k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// KubeDiscoveryService is responsible for Kubernetes Namespace, Service and Deployment discovery
type KubeDiscoveryService struct {
	K8sClient      kubernetes.Interface
	K8sWatchEvents chan model.UpdateItem
	Namespaces     chan model.Namespace
	Services       chan model.Service
	ServicesState  map[model.ServicesStateKey]model.Service
	UpdatesQueue   chan model.UpdateItem
	Errors         chan error
}

// NewKubeDiscoveryService created a new
func NewKubeDiscoveryService(kubeClient *kubernetes.Clientset, state map[model.ServicesStateKey]model.Service, updatesQueue chan model.UpdateItem, errs chan error) *KubeDiscoveryService {
	namespaces := make(chan model.Namespace, 10)
	services := make(chan model.Service, 10)
	watchEvents := make(chan model.UpdateItem, 10)
	return &KubeDiscoveryService{
		K8sClient:      kubeClient,
		K8sWatchEvents: watchEvents,
		Namespaces:     namespaces,
		Services:       services,
		ServicesState:  state,
		UpdatesQueue:   updatesQueue,
		Errors:         errs,
	}
}

// ReloadServiceConfigs gets the latest Namespace and Service configs from k8s and
// persists them
func (d *KubeDiscoveryService) ReloadServiceConfigs(reloadQueue chan uuid.UUID, mgoRepo *db.MongoRepository) {

	for reqID := range reloadQueue {

		log.Infof("reloading k8s configs for request %v", reqID.String())
		go func(errs chan error) {
			for e := range errs {
				log.Printf("ERROR: %v", e)
			}
		}(d.Errors)

		servicesUpdater := db.NewK8sServicesConfigUpdater(d.Services, mgoRepo.WithNewSession())
		namespacesUpdater := db.NewK8sNamespacesConfigUpdater(d.Namespaces, mgoRepo.WithNewSession())

		go func() {
			namespacesUpdater.UpsertNamespaceConfigs()
		}()

		go func() {
			servicesUpdater.UpsertServiceConfigs()
		}()

		go func() {
			d.GetClusterHealthcheckConfig()
		}()
	}
}

// UpdateDeployments processes model.UpdateItem before adding them to the UpdatesQueue
func (d *KubeDiscoveryService) UpdateDeployments() {

	for watchEvent := range d.K8sWatchEvents {
		if string(watchEvent.Type) == string(watch.Error) {
			log.Errorf("k8s watch event returned error")
			continue
		}
		switch v := watchEvent.Object.(type) {
		case model.Deployment:
			d.UpdatesQueue <- model.UpdateItem{Type: string(watchEvent.Type), Object: v}
		default:
			fmt.Printf("unsupported type %T!\n", v)
		}
	}
}

// WatchDeployments sets up Kubernetes API watchers which listen for changes to objects within a
// list of provided namespaces
func (d *KubeDiscoveryService) WatchDeployments(namespaces []string) {

	go d.UpdateDeployments()

	for _, namespace := range namespaces {

		watcher, err := d.K8sClient.AppsV1().Deployments(namespace).Watch(metav1.ListOptions{ResourceVersion: "0"})
		if err != nil {
			panic(err)
		}
		log.Debugf("watching deployments for namespace %s", namespace)

		for event := range watcher.ResultChan() {
			k8sDeployment, ok := event.Object.(*appsv1.Deployment)
			if !ok {
				log.Fatal("unexpected type")
			}

			log.Debugf("received event of type %s for service %s in namespace %s", string(event.Type), k8sDeployment.Spec.Template.Labels["app"], k8sDeployment.Namespace)

			var deployment model.Deployment
			deployment.Namespace = k8sDeployment.Namespace
			deployment.Service = k8sDeployment.Spec.Template.Labels["app"]
			deployment.DesiredReplicas = *k8sDeployment.Spec.Replicas

			servicesStateKey := model.ServicesStateKey{Namespace: deployment.Namespace, Service: deployment.Service}

			log.WithFields(log.Fields{
				"service":   deployment.Service,
				"namespace": deployment.Namespace,
			}).Debugf("searching for service in state object with key: %+v", servicesStateKey)

			_, exists := d.ServicesState[servicesStateKey]

			if exists {
				log.WithFields(log.Fields{
					"service":   deployment.Service,
					"namespace": deployment.Namespace,
				}).Debug("found service in state object")
				serviceState := d.ServicesState[servicesStateKey]
				if serviceState.Deployment.DesiredReplicas != deployment.DesiredReplicas {
					log.WithFields(log.Fields{
						"service":   deployment.Service,
						"namespace": deployment.Namespace,
					}).Debugf("event of type %s received - service state updated (change in deployment)", string(event.Type))
					serviceState.Deployment.DesiredReplicas = deployment.DesiredReplicas
					d.ServicesState[servicesStateKey] = serviceState
					d.K8sWatchEvents <- model.UpdateItem{Type: string(event.Type), Object: deployment}
					continue
				}
				log.WithFields(log.Fields{
					"service":   deployment.Service,
					"namespace": deployment.Namespace,
				}).Debugf("event of type %s received - service state unchanged (no change in deployment)", string(event.Type))
				continue
			}
			log.WithFields(log.Fields{
				"service":   deployment.Service,
				"namespace": deployment.Namespace,
			}).Debug("service not found service in state object")

			// TODO it's service we don't know about (probably new) and we need to do something about that

		}
	}
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
func (d *KubeDiscoveryService) GetClusterHealthcheckConfig() {

	log.Info("loading namespace and service annotations")
	defaultAnnotations := model.HealthAnnotations{EnableScrape: constants.DefaultEnableScrape, Port: constants.DefaultPort}

	namespaces, err := d.K8sClient.Core().Namespaces().List(metav1.ListOptions{})
	if err != nil {
		select {
		case d.Errors <- fmt.Errorf("Could not get namespaces via kubernetes api 1: (%v)", err):
		default:
		}
		return
	}

	for _, n := range namespaces.Items {
		namespaceAnnotations, err := getHealthAnnotations(n)
		if err != nil {
			select {
			case d.Errors <- fmt.Errorf("Could not get namespace annotations via kubernetes api 2: (%v)", err):
			default:
			}
			return
		}

		namespaceAnnotations = overrideParentAnnotations(namespaceAnnotations, defaultAnnotations)

		d.Namespaces <- model.Namespace{
			Name:              n.Name,
			HealthAnnotations: namespaceAnnotations,
		}

		log.Debugf("Added namespace %v to channel\n", n.Name)

		services, err := d.K8sClient.Core().Services(n.Name).List(metav1.ListOptions{})
		if err != nil {
			select {
			case d.Errors <- fmt.Errorf("Could not get services via kubernetes api: (%v)", err):
			default:
			}
			return
		}

		// exclude those services where no pods are intended to run
		deployments, depErr := d.getDeployments(n.Name)
		if depErr != nil {
			log.Errorf("Failed getting deployments, err: %v", depErr)
		}

		for _, svc := range services.Items {

			if _, exists := deployments[svc.Name]; !exists {
				log.Debugf("cannot find deployment for service with name %s", svc.Name)
				continue
			}

			serviceAnnotations, err := getHealthAnnotations(svc)
			if err != nil {
				select {
				case d.Errors <- fmt.Errorf("Could not get service annotations via kubernetes api: (%v)", err):
				default:
				}
				continue
			}
			serviceAnnotations = overrideParentAnnotations(serviceAnnotations, namespaceAnnotations)

			appPort := getAppPortForService(&svc, serviceAnnotations.Port)

			d.Services <- model.Service{
				Name:              svc.Name,
				Namespace:         n.Name,
				HealthcheckURL:    fmt.Sprintf("http://%s.%s:%s/__/health", svc.Name, n.Name, serviceAnnotations.Port),
				HealthAnnotations: serviceAnnotations,
				AppPort:           appPort,
				Deployment:        deployments[svc.Name],
			}
			log.Debugf("Added service %v to channel\n", svc.Name)
		}
	}
}

func (d *KubeDiscoveryService) getDeployments(namespaceName string) (map[string]model.Deployment, error) {
	deploymentList, err := d.K8sClient.ExtensionsV1beta1().Deployments(namespaceName).List(metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve deployments: %v", err.Error())
	}

	deployments := make(map[string]model.Deployment)
	for _, deployment := range deploymentList.Items {
		deployments[deployment.Name] = model.Deployment{
			DesiredReplicas: *deployment.Spec.Replicas,
		}
	}
	return deployments, nil
}

func getHealthAnnotations(k8sObject interface{}) (model.HealthAnnotations, error) {

	switch k8sObject.(type) {
	case corev1.Namespace:
		ns := k8sObject.(corev1.Namespace)
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
	case corev1.Service:
		svc := k8sObject.(corev1.Service)
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

func getAppPortForService(k8sService *corev1.Service, serviceScrapePort string) string {
	servicePorts := k8sService.Spec.Ports
	for _, port := range servicePorts {
		scrapePort, _ := strconv.Atoi(serviceScrapePort)
		if port.Port == int32(scrapePort) {
			if port.TargetPort.StrVal != "" {
				return port.TargetPort.StrVal
			}
			if port.TargetPort.IntVal != 0 {
				return strconv.Itoa(int(port.TargetPort.IntVal))
			}
		}
	}
	return serviceScrapePort
}
