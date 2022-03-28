/*
Copyright 2022.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	cloudv1beta1 "dancingcode.cn/kb-nrr-controller/api/v1beta1"
	"fmt"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// NamespaceResourceReportReconciler reconciles a NamespaceResourceReport object
type NamespaceResourceReportReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=cloud.dancingcode.cn,resources=namespaceresourcereports,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cloud.dancingcode.cn,resources=namespaceresourcereports/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cloud.dancingcode.cn,resources=namespaceresourcereports/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the NamespaceResourceReport object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.11.0/pkg/reconcile

func (r *NamespaceResourceReportReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	_ = log.FromContext(ctx)
	config := ctrl.GetConfigOrDie()
	klog.Info(config)
	// TODO(user): your logic here
	instance := &cloudv1beta1.NamespaceResourceReport{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		klog.Fatal("r.Get error")
	}
	namespaceName := instance.Spec.NamespaceName
	klog.Infoln(namespaceName)

	mc, err := metrics.NewForConfig(config)
	if err != nil {
		klog.Fatalf("metric.NewForConfig error:%s", err.Error())
	}
	// get podMetrics of namespaceName
	podMetrics, err := mc.MetricsV1beta1().PodMetricses(namespaceName).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		klog.Fatal("mc.MetricsV1beta1 error:%s", err.Error())
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
				klog.Fatal("container.Usage error")
			}
			msg := fmt.Sprintf("Container Name: %s \n CPU usage: %d \n Memory usage: %d", container.Name, cpuQuantity, memQuantity)
			fmt.Println(msg)
		}
	}
	klog.Info("===============================================================")
	klog.Infof("All pods in %s \"namespace\" using cpu=[%d], memory=[%.2f MiB]", namespaceName, totalCPU, float64(totalMem)/(float64(1024*1024)))
	klog.Info("===============================================================")

	// update status
	instance.Status.CPU = int(totalCPU)
	instance.Status.Memory = int(totalMem)
	err = r.Status().Update(ctx, instance)
	if err != nil {
		klog.Fatalf("r.Status().Update(ctx, instance) error:%s", err.Error())
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *NamespaceResourceReportReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cloudv1beta1.NamespaceResourceReport{}).
		Complete(r)
}
