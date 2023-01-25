/*
Copyright 2023.

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
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	quotaResource "k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	environmentv1alpha1 "github.com/dunefro/workspace-operator/api/v1alpha1"
)

// WorkspaceReconciler reconciles a Workspace object
type WorkspaceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

//+kubebuilder:rbac:groups=environment.tf.operator.com,resources=workspaces,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=environment.tf.operator.com,resources=workspaces/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=environment.tf.operator.com,resources=workspaces/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Workspace object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.13.0/pkg/reconcile
func (r *WorkspaceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {

	// setting up logging with zap from the controller
	reconcilerLog := ctrl.Log.WithName("reconciler")

	// We create a CR of Workspace and then we query the workspaces across req.NamespacedName
	// The reconciler loop is triggered by a request that is carried out in req
	// The query takes place by req.NamespacedName which contains {Namespace: string, Name: string}
	workspace := &environmentv1alpha1.Workspace{}
	err := r.Get(ctx, req.NamespacedName, workspace)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// If the custom resource is not found then, it usually means that it was deleted or not created
			// In this way, we will stop the reconciliation
			reconcilerLog.Info("Workspace resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		// Error reading the object - requeue the request.
		reconcilerLog.Error(err, "Failed to get workspace")
		return ctrl.Result{}, err
	}
	// If we come here it means error was nil and there is a workspace created.
	// From now we will check whether that workspace created all the required resources or not.

	// Check if the namespace already exists, if not create a new one
	// We create a namespace pointer and check if namespace exists with the name in workspace.Spec.Name
	namespace := &corev1.Namespace{}
	err = r.Get(ctx, types.NamespacedName{Namespace: "", Name: workspace.Spec.Name}, namespace)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new namespace as the namespace is not found
		ns, err := r.namespaceForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new Namespace resource for Workspace")
			return ctrl.Result{}, err
		}

		// we will now create the namespace.
		reconcilerLog.Info(fmt.Sprintf("Creating a new Namespace Namespace.Name %s", ns.Name))
		if err = r.Create(ctx, ns); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new Namespace Namespace.Name %s", ns.Name))
			return ctrl.Result{}, err
		}

		// Namespace created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get Namespace")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// Check if resource quotas for the namespace exists
	// resource-quota name will be Namespace.Name-quota
	resourceQuota := corev1.ResourceQuota{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-quota", workspace.Spec.Name)}, &resourceQuota)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new resourcequota as the resourcequota is not found
		rq, err := r.resourceQuotaForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new ResourceQuota resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of namespace object, we will now create the namespace.
		reconcilerLog.Info(fmt.Sprintf("Creating a new ResourceQuota ResourceQuota.Name %s", rq.Name))
		if err = r.Create(ctx, rq); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new ResourceQuota ResourceQuota.Name %s", rq.Name))
			return ctrl.Result{}, err
		}

		// ResourceQuota created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get ResourceQuota")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// Check if roles are created or not
	// 1. Admin role
	adminRole := rbacv1.Role{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-admin", workspace.Spec.Name)}, &adminRole)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new role as the admin role is not found
		ar, err := r.adminRoleForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new admin Role resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of admin Role object, we will now create the admin Role.
		reconcilerLog.Info(fmt.Sprintf("Creating a new Admin Role Role.Name %s", ar.Name))
		if err = r.Create(ctx, ar); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new Admin Role Role.Name %s", ar.Name))
			return ctrl.Result{}, err
		}

		// Admin Role created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get Admin role")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}
	// 2. Editor role
	editorRole := rbacv1.Role{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-editor", workspace.Spec.Name)}, &editorRole)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new role as the editor role is not found
		er, err := r.editorRoleForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new editor Role resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of editor Role object, we will now create the editor Role.
		reconcilerLog.Info(fmt.Sprintf("Creating a new Editor Role Role.Name %s", er.Name))
		if err = r.Create(ctx, er); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new Editor Role Role.Name %s", er.Name))
			return ctrl.Result{}, err
		}

		// Editor Role created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get Editor role")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}
	// 3. Viewer role
	viewerRole := rbacv1.Role{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-viewer", workspace.Spec.Name)}, &viewerRole)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new role as the viewer role is not found
		vr, err := r.viewerRoleForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new viewer Role resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of viewer Role object, we will now create the viewer Role.
		reconcilerLog.Info(fmt.Sprintf("Creating a new Viewer Role Role.Name %s", vr.Name))
		if err = r.Create(ctx, vr); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new Viewer Role Role.Name %s", vr.Name))
			return ctrl.Result{}, err
		}

		// Viewer Role created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get Viewer role")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// Check rolebindings
	// 1. AdminRoleBinding
	adminRoleBinding := rbacv1.RoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-admin-rb", workspace.Spec.Name)}, &adminRoleBinding)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new rolebinding
		arb, err := r.adminRoleBindingForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new admin RoleBinding resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of admin RoleBinding object, we will now create the admin RoleBinding.
		reconcilerLog.Info(fmt.Sprintf("Creating a new Admin RoleBinding RoleBinding.Name %s", arb.Name))
		if err = r.Create(ctx, arb); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new Admin RoleBinding RoleBinding.Name %s", arb.Name))
			return ctrl.Result{}, err
		}

		// Admin Role Binding created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get Admin RoleBinding")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// EditorRoleBinding
	editorRoleBinding := rbacv1.RoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-editor-rb", workspace.Spec.Name)}, &editorRoleBinding)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new rolebinding
		erb, err := r.editorRoleBindingForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new editor RoleBinding resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of editor RoleBinding object, we will now create the editor RoleBinding.
		reconcilerLog.Info(fmt.Sprintf("Creating a new editor RoleBinding RoleBinding.Name %s", erb.Name))
		if err = r.Create(ctx, erb); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new editor RoleBinding RoleBinding.Name %s", erb.Name))
			return ctrl.Result{}, err
		}

		// Editor Role Binding created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get editor RoleBinding")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// ViewerRoleBinding
	viewerRoleBinding := rbacv1.RoleBinding{}
	err = r.Get(ctx, types.NamespacedName{Namespace: workspace.Spec.Name, Name: fmt.Sprintf("%s-viewer-rb", workspace.Spec.Name)}, &viewerRoleBinding)
	if err != nil && apierrors.IsNotFound(err) {
		// Define a new rolebinding
		erb, err := r.viewerRoleBindingForWorkspace(workspace)
		if err != nil {
			reconcilerLog.Error(err, "Failed to define new viewer RoleBinding resource for Workspace")
			return ctrl.Result{}, err
		}

		// When we create a pointer of viewer RoleBinding object, we will now create the viewer RoleBinding.
		reconcilerLog.Info(fmt.Sprintf("Creating a new viewer RoleBinding RoleBinding.Name %s", erb.Name))
		if err = r.Create(ctx, erb); err != nil {
			reconcilerLog.Error(err, fmt.Sprintf("Error creating a new viewer RoleBinding RoleBinding.Name %s", erb.Name))
			return ctrl.Result{}, err
		}

		// Viewer Role Binding created successfully
		// We will requeue the reconciliation so that we can ensure the state
		// and move forward for the next operations
		return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
	} else if err != nil {
		reconcilerLog.Error(err, "Failed to get viewer RoleBinding")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}

	// Check if Workspace labels are updated
	workspaceLabels := workspace.Spec.Labels
	namespaceLabels := namespace.ObjectMeta.Labels
	resourceQuotaLabels := resourceQuota.ObjectMeta.Labels
	adminRoleLabels := adminRole.ObjectMeta.Labels
	editorRoleLabels := editorRole.ObjectMeta.Labels
	viewerRoleLabels := viewerRole.ObjectMeta.Labels
	// Check for namespace labels
	for k, v := range workspaceLabels {
		value, ok := namespaceLabels[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Labels not same for Namespace.Name %s", workspace.Spec.Name))
			namespace.ObjectMeta.Labels = workspaceLabels
			if err := r.Update(ctx, namespace); err != nil {
				reconcilerLog.Error(err, "Failed to update Namespace.ObjectMeta.Labels for Namespace")
				return ctrl.Result{}, err
			}
		}
	}
	// Check for resourceQuota labels
	for k, v := range workspaceLabels {
		value, ok := resourceQuotaLabels[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Labels not same for ResourceQuota.Name %s in Namespace.Name %s", fmt.Sprintf("%s-quota", workspace.Spec.Name), workspace.Spec.Name))
			resourceQuota.ObjectMeta.Labels = workspaceLabels
			if err := r.Update(ctx, &resourceQuota); err != nil {
				reconcilerLog.Error(err, "Failed to update ResourceQuota.ObjectMeta.Labels for ResourceQuota")
				return ctrl.Result{}, err
			}
		}
	}
	// Check for adminRole labels
	for k, v := range workspaceLabels {
		value, ok := adminRoleLabels[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Labels not same for admin Role.Name %s in Namespace.Name %s", fmt.Sprintf("%s-admin", workspace.Spec.Name), workspace.Spec.Name))
			adminRole.ObjectMeta.Labels = workspaceLabels
			if err := r.Update(ctx, &adminRole); err != nil {
				reconcilerLog.Error(err, "Failed to update adminRole.ObjectMeta.Labels")
				return ctrl.Result{}, err
			}
		}
	}
	// Check for editorRole labels
	for k, v := range workspaceLabels {
		value, ok := editorRoleLabels[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Labels not same for editor Role.Name %s in Namespace.Name %s", fmt.Sprintf("%s-editor", workspace.Spec.Name), workspace.Spec.Name))
			editorRole.ObjectMeta.Labels = workspaceLabels
			if err := r.Update(ctx, &editorRole); err != nil {
				reconcilerLog.Error(err, "Failed to update editorRole.ObjectMeta.Labels")
				return ctrl.Result{}, err
			}
		}
	}
	// Check for viewerRole labels
	for k, v := range workspaceLabels {
		value, ok := viewerRoleLabels[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Labels not same for viewer Role.Name %s in Namespace.Name %s", fmt.Sprintf("%s-viewer", workspace.Spec.Name), workspace.Spec.Name))
			viewerRole.ObjectMeta.Labels = workspaceLabels
			if err := r.Update(ctx, &viewerRole); err != nil {
				reconcilerLog.Error(err, "Failed to update viewerRole.ObjectMeta.Labels")
				return ctrl.Result{}, err
			}
		}
	}

	// leaving label checking for RoleBindings

	// Check if Workspace annotations are updated
	workspaceAnnotations := workspace.Spec.Annotations
	namespaceAnnotations := namespace.ObjectMeta.Annotations
	resourceQuotaAnnotations := resourceQuota.ObjectMeta.Annotations
	// Check for namespace annotations
	for k, v := range workspaceAnnotations {
		value, ok := namespaceAnnotations[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Annotations not same for Namespace.Name %s", workspace.Spec.Name))
			namespace.ObjectMeta.Annotations = workspaceAnnotations
			if err := r.Update(ctx, namespace); err != nil {
				reconcilerLog.Error(err, "Failed to update Namespace.ObjectMeta.Annotations for Namespace")
				return ctrl.Result{}, err
			}
		}
	}
	// Check for resourceQuota annotations
	for k, v := range workspaceAnnotations {
		value, ok := resourceQuotaAnnotations[k]
		if !ok || value != v {
			reconcilerLog.Info(fmt.Sprintf("Annotations not same for ResourceQuota.Name %s in Namespace.Name %s", fmt.Sprintf("%s-quota", workspace.Spec.Name), workspace.Spec.Name))
			resourceQuota.ObjectMeta.Annotations = workspaceAnnotations
			if err := r.Update(ctx, &resourceQuota); err != nil {
				reconcilerLog.Error(err, "Failed to update ResourceQuota.ObjectMeta.Annotations for ResourceQuota")
				return ctrl.Result{}, err
			}
		}
	}

	// check if admin rolebindings has right user
	adminUserName := workspace.Spec.Users.Admin
	if adminUserName != adminRoleBinding.Subjects[0].Name {
		reconcilerLog.Info(fmt.Sprintf("User not same for admin RoleBinding %s in Namespace.Name %s", fmt.Sprintf("%s-admin-rb", workspace.Spec.Name), workspace.Spec.Name))
		adminRoleBinding.Subjects[0].Name = adminUserName
		if err := r.Update(ctx, &adminRoleBinding); err != nil {
			reconcilerLog.Error(err, "Failed to update admin RoleBinding")
			return ctrl.Result{}, err
		}
	}

	// check if editor rolebindings has right user
	editorUserName := workspace.Spec.Users.Editor
	if editorUserName != editorRoleBinding.Subjects[0].Name {
		reconcilerLog.Info(fmt.Sprintf("User not same for editor RoleBinding %s in Namespace.Name %s", fmt.Sprintf("%s-editor-rb", workspace.Spec.Name), workspace.Spec.Name))
		editorRoleBinding.Subjects[0].Name = editorUserName
		if err := r.Update(ctx, &editorRoleBinding); err != nil {
			reconcilerLog.Error(err, "Failed to update editor RoleBinding")
			return ctrl.Result{}, err
		}
	}

	// check if viewer rolebindings has right user
	viewerUserName := workspace.Spec.Users.Viewer
	if viewerUserName != viewerRoleBinding.Subjects[0].Name {
		reconcilerLog.Info(fmt.Sprintf("User not same for viewer RoleBinding %s in Namespace.Name %s", fmt.Sprintf("%s-viewer-rb", workspace.Spec.Name), workspace.Spec.Name))
		viewerRoleBinding.Subjects[0].Name = viewerUserName
		if err := r.Update(ctx, &viewerRoleBinding); err != nil {
			reconcilerLog.Error(err, "Failed to update viewer RoleBinding")
			return ctrl.Result{}, err
		}
	}

	// Check if resourceQuota has right cpu, memory and disk
	// 1. checking memory
	workspaceMemory := workspace.Spec.Resources.Memory
	workspaceMemoryQuantity, err := quotaResource.ParseQuantity(workspaceMemory)
	if err != nil {
		reconcilerLog.Error(err, "Not able to parse workspace.Spec.Resources.Memory")
		return ctrl.Result{}, err
	}
	// comparing if Memory in workspace matches Memory in resourceQuota
	if workspaceMemoryQuantity.Cmp(resourceQuota.Spec.Hard[corev1.ResourceMemory]) != 0 {
		reconcilerLog.Info(fmt.Sprintf("Memory not same for ResourceQuota.Name %s in Namespace.Name %s", fmt.Sprintf("%s-quota", workspace.Spec.Name), workspace.Spec.Name))
		resourceQuota.Spec.Hard[corev1.ResourceMemory] = workspaceMemoryQuantity
		if err := r.Update(ctx, &resourceQuota); err != nil {
			reconcilerLog.Error(err, "Failed to update resourceQuota.Spec.Hard[corev1.ResourceMemory]")
			return ctrl.Result{}, err
		}
	}
	// 2. checking CPU
	workspaceCPU := workspace.Spec.Resources.CPU
	workspaceCPUQuantity, err := quotaResource.ParseQuantity(workspaceCPU)
	if err != nil {
		reconcilerLog.Error(err, "Not able to parse workspace.Spec.Resources.Memory")
		return ctrl.Result{}, err
	}
	// comparing if CPU in workspace matches CPU in resourceQuota
	if workspaceCPUQuantity.Cmp(resourceQuota.Spec.Hard[corev1.ResourceCPU]) != 0 {
		reconcilerLog.Info(fmt.Sprintf("CPU not same for ResourceQuota.Name %s in Namespace.Name %s", fmt.Sprintf("%s-quota", workspace.Spec.Name), workspace.Spec.Name))
		resourceQuota.Spec.Hard[corev1.ResourceCPU] = workspaceCPUQuantity
		if err := r.Update(ctx, &resourceQuota); err != nil {
			reconcilerLog.Error(err, "Failed to update resourceQuota.Spec.Hard[corev1.ResourceCPU] for ResourceQuota")
			return ctrl.Result{}, err
		}
	}
	// 3. checking disk size
	workspaceDisk := workspace.Spec.Resources.Disk
	workspaceDiskQuantity, err := quotaResource.ParseQuantity(workspaceDisk)
	if err != nil {
		reconcilerLog.Error(err, "Not able to parse workspace.Spec.Resources.Disk")
		return ctrl.Result{}, err
	}
	// comparing if Disk in workspace matches Disk in resourceQuota
	if workspaceDiskQuantity.Cmp(resourceQuota.Spec.Hard[corev1.ResourceRequestsStorage]) != 0 {
		reconcilerLog.Info(fmt.Sprintf("Disk not same for ResourceQuota.Name %s in Namespace.Name %s", fmt.Sprintf("%s-quota", workspace.Spec.Name), workspace.Spec.Name))
		resourceQuota.Spec.Hard[corev1.ResourceRequestsStorage] = workspaceDiskQuantity
		if err := r.Update(ctx, &resourceQuota); err != nil {
			reconcilerLog.Error(err, "Failed to update resourceQuota.Spec.Hard[corev1.ResourceRequestsStorage] for ResourceQuota")
			return ctrl.Result{}, err
		}
	}

	// This will force the check for controller after every 5 seconds
	// This is done to maintain the namespace state, for e.g. if the namespace is deleted
	// it should be created again to maintain the state of workspace
	return ctrl.Result{RequeueAfter: 3 * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *WorkspaceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&environmentv1alpha1.Workspace{}).
		Complete(r)
}

// Namespace for Workspace
func (r *WorkspaceReconciler) namespaceForWorkspace(workspace *environmentv1alpha1.Workspace) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name:        workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Spec: corev1.NamespaceSpec{
			Finalizers: []corev1.FinalizerName{corev1.FinalizerKubernetes},
		},
	}
	if err := ctrl.SetControllerReference(workspace, ns, r.Scheme); err != nil {
		return nil, err
	}
	return ns, nil
}

// ResourceQuota for Workspace
func (r *WorkspaceReconciler) resourceQuotaForWorkspace(workspace *environmentv1alpha1.Workspace) (*corev1.ResourceQuota, error) {
	cpu, err := r.resourceQuotaCPUForWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	memory, err := r.resourceQuotaMemoryForWorkspace(workspace)
	if err != nil {
		return nil, err
	}
	disk, err := r.resourceQuotaStorageForWorkspace(workspace)
	if err != nil {
		return nil, err
	}

	rq := &corev1.ResourceQuota{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-quota", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Spec: corev1.ResourceQuotaSpec{
			Hard: map[corev1.ResourceName]quotaResource.Quantity{
				corev1.ResourceCPU:             *cpu,
				corev1.ResourceMemory:          *memory,
				corev1.ResourceRequestsStorage: *disk,
			},
		},
	}
	if err := ctrl.SetControllerReference(workspace, rq, r.Scheme); err != nil {
		return nil, err
	}
	return rq, nil
}

// converts the string to Quantity
func (r *WorkspaceReconciler) resourceQuotaCPUForWorkspace(workspace *environmentv1alpha1.Workspace) (*quotaResource.Quantity, error) {
	cpu, err := quotaResource.ParseQuantity(workspace.Spec.Resources.CPU)
	if err != nil {
		return nil, err
	}
	return &cpu, nil
}

func (r *WorkspaceReconciler) resourceQuotaMemoryForWorkspace(workspace *environmentv1alpha1.Workspace) (*quotaResource.Quantity, error) {
	memory, err := quotaResource.ParseQuantity(workspace.Spec.Resources.Memory)
	if err != nil {
		return nil, err
	}
	return &memory, nil
}

func (r *WorkspaceReconciler) resourceQuotaStorageForWorkspace(workspace *environmentv1alpha1.Workspace) (*quotaResource.Quantity, error) {
	disk, err := quotaResource.ParseQuantity(workspace.Spec.Resources.Disk)
	if err != nil {
		return nil, err
	}
	return &disk, nil
}

// Admin role for Workspace
func (r *WorkspaceReconciler) adminRoleForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.Role, error) {

	adminRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-admin", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
					"delete",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"*",
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(workspace, adminRole, r.Scheme); err != nil {
		return nil, err
	}
	return adminRole, nil
}

// Editor role for Workspace
func (r *WorkspaceReconciler) editorRoleForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.Role, error) {

	editorRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-editor", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"list",
					"watch",
					"create",
					"update",
					"patch",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"*",
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(workspace, editorRole, r.Scheme); err != nil {
		return nil, err
	}
	return editorRole, nil
}

// Viewer role for Workspace
func (r *WorkspaceReconciler) viewerRoleForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.Role, error) {

	viewerRole := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-viewer", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs: []string{
					"get",
					"list",
					"watch",
				},
				APIGroups: []string{
					"",
				},
				Resources: []string{
					"*",
				},
			},
		},
	}
	if err := ctrl.SetControllerReference(workspace, viewerRole, r.Scheme); err != nil {
		return nil, err
	}
	return viewerRole, nil
}

// Admin role Binding for Workspace
func (r *WorkspaceReconciler) adminRoleBindingForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.RoleBinding, error) {

	adminRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-admin-rb", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     workspace.Spec.Users.Admin,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     fmt.Sprintf("%s-admin", workspace.Spec.Name),
		},
	}
	if err := ctrl.SetControllerReference(workspace, adminRoleBinding, r.Scheme); err != nil {
		return nil, err
	}
	return adminRoleBinding, nil
}

// Editor role Binding for Workspace
func (r *WorkspaceReconciler) editorRoleBindingForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.RoleBinding, error) {

	editorRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-editor-rb", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     workspace.Spec.Users.Editor,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     fmt.Sprintf("%s-editor", workspace.Spec.Name),
		},
	}
	if err := ctrl.SetControllerReference(workspace, editorRoleBinding, r.Scheme); err != nil {
		return nil, err
	}
	return editorRoleBinding, nil
}

// Viewer role Binding for Workspace
func (r *WorkspaceReconciler) viewerRoleBindingForWorkspace(workspace *environmentv1alpha1.Workspace) (*rbacv1.RoleBinding, error) {

	viewerRoleBinding := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:        fmt.Sprintf("%s-viewer-rb", workspace.Spec.Name),
			Namespace:   workspace.Spec.Name,
			Labels:      workspace.Spec.Labels,
			Annotations: workspace.Spec.Annotations,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:     "User",
				Name:     workspace.Spec.Users.Viewer,
				APIGroup: "rbac.authorization.k8s.io",
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "Role",
			APIGroup: "rbac.authorization.k8s.io",
			Name:     fmt.Sprintf("%s-viewer", workspace.Spec.Name),
		},
	}
	if err := ctrl.SetControllerReference(workspace, viewerRoleBinding, r.Scheme); err != nil {
		return nil, err
	}
	return viewerRoleBinding, nil
}
