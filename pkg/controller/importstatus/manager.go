// Copyright (c) Red Hat, Inc.
// Copyright Contributors to the Open Cluster Management project

package importstatus

import (
	"strings"

	"github.com/stolostron/managedcluster-import-controller/pkg/constants"
	"github.com/stolostron/managedcluster-import-controller/pkg/helpers"
	"github.com/stolostron/managedcluster-import-controller/pkg/source"
	"k8s.io/apimachinery/pkg/api/equality"

	clusterv1 "open-cluster-management.io/api/cluster/v1"
	workv1 "open-cluster-management.io/api/work/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	runtimesource "sigs.k8s.io/controller-runtime/pkg/source"
)

const controllerName = "import-status-controller"

// Add creates a new manifestwork controller and adds it to the Manager.
// The Manager will set fields on the Controller and Start it when the Manager is Started.
func Add(mgr manager.Manager, clientHolder *helpers.ClientHolder, informerHolder *source.InformerHolder) (string, error) {
	c, err := controller.New(controllerName, mgr, controller.Options{
		Reconciler: &ReconcileImportStatus{
			client:     clientHolder.RuntimeClient,
			kubeClient: clientHolder.KubeClient,
			workClient: clientHolder.WorkClient,
			recorder:   helpers.NewEventRecorder(clientHolder.KubeClient, controllerName),
		},
		MaxConcurrentReconciles: helpers.GetMaxConcurrentReconciles(),
	})
	if err != nil {
		return controllerName, err
	}

	if err := c.Watch(
		source.NewKlusterletWorkSource(informerHolder.KlusterletWorkInformer),
		&source.ManagedClusterResourceEventHandler{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return false },
			CreateFunc:  func(e event.CreateEvent) bool { return true },
			DeleteFunc:  func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool {
				workName := e.ObjectNew.GetName()
				// for update event, only watch klusterlet manifest works
				if !strings.HasSuffix(workName, constants.KlusterletCRDsSuffix) &&
					!strings.HasSuffix(workName, constants.KlusterletSuffix) {
					return false
				}

				new, okNew := e.ObjectNew.(*workv1.ManifestWork)
				old, okOld := e.ObjectOld.(*workv1.ManifestWork)
				if okNew && okOld {
					return !equality.Semantic.DeepEqual(new.Status.Conditions, old.Status.Conditions)
				}

				return false
			},
		}),
	); err != nil {
		return controllerName, err
	}

	if err := c.Watch(
		&runtimesource.Kind{Type: &clusterv1.ManagedCluster{}},
		&handler.EnqueueRequestForObject{},
		predicate.Predicate(predicate.Funcs{
			GenericFunc: func(e event.GenericEvent) bool { return isDefaultModeObject(e.Object) },
			DeleteFunc:  func(e event.DeleteEvent) bool { return isDefaultModeObject(e.Object) },
			CreateFunc:  func(e event.CreateEvent) bool { return isDefaultModeObject(e.Object) },
			UpdateFunc:  func(e event.UpdateEvent) bool { return isDefaultModeObject(e.ObjectNew) },
		})); err != nil {
		return controllerName, err
	}

	return controllerName, nil
}

func isDefaultModeObject(object client.Object) bool {
	return !strings.EqualFold(object.GetAnnotations()[constants.KlusterletDeployModeAnnotation], constants.KlusterletDeployModeHosted)
}
