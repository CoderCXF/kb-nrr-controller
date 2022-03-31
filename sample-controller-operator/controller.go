package main

import (
	"context"
	"fmt"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	appsinformers "k8s.io/client-go/informers/apps/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	appslisters "k8s.io/client-go/listers/apps/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"

	samplev1alpha1 "nrr-controller/pkg/apis/cloud/v1"
	clientset "nrr-controller/pkg/client/clientset/versioned"
	samplescheme "nrr-controller/pkg/client/clientset/versioned/scheme"
	informers "nrr-controller/pkg/client/informers/externalversions/cloud/v1"
	listers "nrr-controller/pkg/client/listers/cloud/v1"
)

const controllerAgentName = "nrr-controller"

const (
	// SuccessSynced is used as part of the Event 'reason' when a Foo is synced
	SuccessSynced = "Synced"
	// ErrResourceExists is used as part of the Event 'reason' when a Foo fails
	// to sync due to a Deployment of the same name already existing.
	ErrResourceExists = "ErrResourceExists"

	// MessageResourceExists is the message used for Events when a resource
	// fails to sync due to a Deployment already existing
	MessageResourceExists = "Resource %q already exists and is not managed by Foo"
	// MessageResourceSynced is the message used for an Event fired when a Foo
	// is synced successfully
	MessageResourceSynced = "Foo synced successfully"
)

// Controller is the controller implementation for Foo resources
type Controller struct {
	// kubeclientset is a standard kubernetes clientset
	kubeclientset kubernetes.Interface
	// nrrclientset is a clientset for our own API group
	nrrclientset clientset.Interface
	mc           metrics.Interface

	deploymentsLister appslisters.DeploymentLister
	deploymentsSynced cache.InformerSynced
	foosLister        listers.NamespaceResourceReportLister
	foosSynced        cache.InformerSynced

	// workqueue is a rate limited work queue. This is used to queue work to be
	// processed instead of performing it as soon as a change happens. This
	// means we can ensure we only process a fixed amount of resources at a
	// time, and makes it easy to ensure we are never processing the same item
	// simultaneously in two different workers.
	workqueue workqueue.RateLimitingInterface
	// recorder is an event recorder for recording Event resources to the
	// Kubernetes API.
	recorder record.EventRecorder
}

// NewController returns a new sample controller
// NewController(
// 1. kubeclientset-->main: kubeClient, err := clientsetkubernetes.NewForConfig(cfg)
// 2. nrrclientset -->main: sampleClient, err := clientset.NewForConfig(cfg)
// 3. deploymentInformer-->main: kubeInformerFactory := kubeinformers.NewSharedInformerFactory(kubeClient, time.Second*30).Apps().V1().Deployments()
// 4. fooInformer-->main: exampleInformerFactory := informers.NewSharedInformerFactory(exampleClient, time.Second*30).samplecontroller..V1alpha1().Foos()
//)
func NewController(
	kubeclientset kubernetes.Interface,
	nrrclientset clientset.Interface,
	deploymentInformer appsinformers.DeploymentInformer,
	fooInformer informers.NamespaceResourceReportInformer,
	mc metrics.Interface) *Controller {

	// Create event broadcaster
	// Add sample-controller types to the default Kubernetes Scheme so Events can be
	// logged for sample-controller types.
	utilruntime.Must(samplescheme.AddToScheme(scheme.Scheme))
	klog.V(4).Info("Creating event broadcaster")
	eventBroadcaster := record.NewBroadcaster()
	eventBroadcaster.StartStructuredLogging(0)
	eventBroadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeclientset.CoreV1().Events("")})
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: controllerAgentName})

	controller := &Controller{
		kubeclientset:     kubeclientset,
		nrrclientset:      nrrclientset,
		mc:                mc,
		deploymentsLister: deploymentInformer.Lister(),
		deploymentsSynced: deploymentInformer.Informer().HasSynced,
		foosLister:        fooInformer.Lister(),
		foosSynced:        fooInformer.Informer().HasSynced,
		workqueue:         workqueue.NewNamedRateLimitingQueue(workqueue.DefaultControllerRateLimiter(), "Foos"),
		recorder:          recorder,
	}

	klog.Info("Setting up event handlers")
	// Set up an event handler for when Foo resources change
	fooInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: controller.enqueueFoo,
		UpdateFunc: func(old, new interface{}) {
			controller.enqueueFoo(new)
		},
		DeleteFunc: controller.enqueueVirtualMachineForDelete,
	})

	return controller
}

// Run will set up the event handlers for types we are interested in, as well
// as syncing informer caches and starting workers. It will block until stopCh
// is closed, at which point it will shutdown the workqueue and wait for
// workers to finish processing their current work items.
func (c *Controller) Run(workers int, stopCh <-chan struct{}) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	// Start the informer factories to begin populating the informer caches
	klog.Info("Starting Foo controller")

	// Wait for the caches to be synced before starting workers
	klog.Info("Waiting for informer caches to sync")
	if ok := cache.WaitForCacheSync(stopCh, c.foosSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}

	klog.Info("Starting workers")
	// Launch two workers to process Foo resources
	for i := 0; i < workers; i++ {
		go wait.Until(c.runWorker, time.Second, stopCh)
	}

	klog.Info("Started workers")
	<-stopCh
	klog.Info("Shutting down workers")

	return nil
}

// runWorker is a long-running function that will continually call the
// processNextWorkItem function in order to read and process a message on the
// workqueue.
func (c *Controller) runWorker() {
	for c.processNextWorkItem() {
	}
}

// processNextWorkItem will read a single work item off the workqueue and
// attempt to process it, by calling the syncHandler.
func (c *Controller) processNextWorkItem() bool {
	obj, shutdown := c.workqueue.Get()

	if shutdown {
		return false
	}

	// We wrap this block in a func so we can defer c.workqueue.Done.
	err := func(obj interface{}) error {
		// We call Done here so the workqueue knows we have finished
		// processing this item. We also must remember to call Forget if we
		// do not want this work item being re-queued. For example, we do
		// not call Forget if a transient error occurs, instead the item is
		// put back on the workqueue and attempted again after a back-off
		// period.
		defer c.workqueue.Done(obj)
		var key string
		var ok bool
		// We expect strings to come off the workqueue. These are of the
		// form namespace/name. We do this as the delayed nature of the
		// workqueue means the items in the informer cache may actually be
		// more up to date that when the item was initially put onto the
		// workqueue.
		if key, ok = obj.(string); !ok {
			// As the item in the workqueue is actually invalid, we call
			// Forget here else we'd go into a loop of attempting to
			// process a work item that is invalid.
			c.workqueue.Forget(obj)
			utilruntime.HandleError(fmt.Errorf("expected string in workqueue but got %#v", obj))
			return nil
		}
		// Run the syncHandler, passing it the namespace/name string of the
		// Foo resource to be synced.
		if err := c.syncHandler(key); err != nil {
			// Put the item back on the workqueue to handle any transient errors.
			// FIXME: my version is no: c.workqueue.AddRateLimited(key)
			c.workqueue.AddRateLimited(key)
			return fmt.Errorf("error syncing '%s': %s, requeuing", key, err.Error())
		}
		// Finally, if no error occurs we Forget this item so it does not
		// get queued again until another change happens.
		c.workqueue.Forget(obj)
		klog.Infof("Successfully synced '%s'", key)
		return nil
	}(obj)

	if err != nil {
		utilruntime.HandleError(err)
		return true
	}

	return true
}

// syncHandler compares the actual state with the desired, and attempts to
// converge the two. It then updates the Status block of the Foo resource
// with the current status of the resource.
func (c *Controller) syncHandler(key string) error { // key=example-nrr, no namespace info
	//foo, err := c.foosLister.NamespaceResourceReports(namespace).Get(name)
	fooList, err := c.foosLister.List(labels.Everything())
	if err != nil {
		// The Foo resource may no longer exist, in which case we stop
		// processing.
		if errors.IsNotFound(err) {
			utilruntime.HandleError(fmt.Errorf("foo '%s' in work queue no longer exists", key))
			return nil
		}
		klog.Infof("[NRRCRD] try to process NamespaceResourceReport ...")
		return err
	}
	for _, foo := range fooList {
		klog.Info(foo.Name)

		//TODO: logic here
		namespaceName := foo.Spec.NamespaceName
		if namespaceName == "" {
			// We choose to absorb the error here as the worker would requeue the
			// resource otherwise. Instead, the next time the resource is updated
			// the resource will be queued again.
			klog.Infof("No namespace [ns] exist in Cluster %s", namespaceName)
			utilruntime.HandleError(fmt.Errorf("%s: deployment name must be specified", key))
			return nil
		}
		// get all pods in namespaceName
		podList, err := c.kubeclientset.CoreV1().Pods(namespaceName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Fatal(err)
		}
		for _, pod := range podList.Items {
			klog.Info(pod)
		}
		// get podMetrics of namespaceName
		podMetrics, err := c.mc.MetricsV1beta1().PodMetricses(namespaceName).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			klog.Fatal(err)
		}
		var totalCPU, totalMem int64
		for _, podMetric := range podMetrics.Items {
			podContainers := podMetric.Containers
			for _, container := range podContainers {
				cpuQuantity, ok := container.Usage.Cpu().AsInt64()
				memQuantity, ok := container.Usage.Memory().AsInt64()
				totalCPU += cpuQuantity
				totalMem += memQuantity
				if !ok {
					return nil
				}
				msg := fmt.Sprintf("Container Name: %s \n CPU usage: %d \n Memory usage: %d", container.Name, cpuQuantity, memQuantity)
				fmt.Println(msg)
			}
		}
		klog.Info("===========================")
		klog.Infof("All pods in %s namespace using cpu=%d, memory=%.2f MiB", namespaceName, totalCPU, float64(totalMem)/(float64(1024*1024)))
		klog.Info("===========================")
		// update status
		c.updateFooStatus(foo, int(totalCPU), int(totalMem))

		//if err != nil {
		//	return err
		//}
		c.recorder.Event(foo, corev1.EventTypeNormal, SuccessSynced, MessageResourceSynced)
	}
	return nil
}
func (c *Controller) updateFooStatus(foo *samplev1alpha1.NamespaceResourceReport, totalCPU int, totalMem int) {
	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use DeepCopy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	fooCopy := foo.DeepCopy()
	fooCopy.Status.CPU = totalCPU
	fooCopy.Status.Memory = int(float64(totalMem) / float64(1024*1024))
	// fooCopy.namespaceName = dev
	//klog.Infof("fooCopy.status.cpu=%d, fooCopy.status.memory=%d MiB", fooCopy.Status.CPU, fooCopy.Status.Memory)

	//_, err2 := c.nrrclientset.CloudV1().NamespaceResourceReports(fooCopy.Spec.NamespaceName).UpdateStatus(context.TODO(), fooCopy, metav1.UpdateOptions{})
	//return err2
	//return
}

// enqueueFoo takes a Foo resource and converts it into a namespace/name
// string which is then put onto the work queue. This method should *not* be
// passed resources of any type other than Foo.
func (c *Controller) enqueueFoo(obj interface{}) {
	var key string
	var err error
	if key, err = cache.MetaNamespaceKeyFunc(obj); err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.Add(key)
}

func (c *Controller) enqueueVirtualMachineForDelete(obj interface{}) {
	var key string
	var err error
	key, err = cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
	if err != nil {
		utilruntime.HandleError(err)
		return
	}
	c.workqueue.AddRateLimited(key)
}
