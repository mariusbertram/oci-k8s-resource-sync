/*
Copyright 2025.

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

// Package controller implements Kubernetes controllers for the OCI resource synchronization.
// This package contains the OCISecretReconciler which is responsible for synchronizing
// content from OCI (Open Container Initiative) registries to Kubernetes Secrets.
//
// The controller watches OCISecret custom resources and ensures that the specified
// OCI artifacts are downloaded and their contents are stored in Kubernetes Secrets.
// It handles authentication to OCI registries, tracks changes to artifacts using
// content digests, and updates the target Secrets when the source artifacts change.
package controller

import (
	"context"
	ocisyncv1aplha1 "github.com/mariusbertram/oci-resource-sync-operator/api/v1aplha1"
	"github.com/mariusbertram/oci-resource-sync-operator/internal/orasclient"
	"github.com/mariusbertram/oci-resource-sync-operator/internal/utils"
	v1core "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"time"
)

// OCISecretReconciler reconciles OCISecret custom resources with Kubernetes Secrets.
// It monitors OCISecret resources and ensures that the specified OCI artifacts
// are downloaded and their contents are stored in the target Kubernetes Secrets.
//
// The reconciler handles:
// - Authentication to OCI registries using pull secrets
// - Downloading artifacts from OCI registries
// - Creating and updating target Secrets with the artifact contents
// - Filtering files based on the OCISecret specification
// - Tracking changes to artifacts using content digests
type OCISecretReconciler struct {
	// Client is a Kubernetes client for interacting with the API server
	client.Client
	// Scheme provides runtime type information for API objects
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=oci-sync.brtrm.de,resources=ocisecrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=oci-sync.brtrm.de,resources=ocisecrets/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=oci-sync.brtrm.de,resources=ocisecrets/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// The Reconcile function implements the controller's main logic:
// 1. Fetch the OCISecret resource being reconciled
// 2. Get the pull secret for OCI registry authentication (if specified)
// 3. Get the digest of the OCI artifact to detect changes
// 4. Create or update the target Secret with the artifact contents
// 5. Schedule the next reconciliation
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/reconcile
func (r *OCISecretReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Get a logger from the context
	logger := log.FromContext(ctx)

	// Step 1: Fetch the OCISecret resource being reconciled
	OCIsecret := &ocisyncv1aplha1.OCISecret{}
	err := r.Get(ctx, req.NamespacedName, OCIsecret)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// The OCISecret resource has been deleted, nothing to do
			logger.Info("OCISecret resource not found.")
			return ctrl.Result{}, nil
		}
		// Error reading the object
		logger.Error(err, "Failed to get OCISecret.")
		return ctrl.Result{}, err
	}

	// Step 2: Get the pull secret for OCI registry authentication (if specified)
	var secretData string
	OCIPullSecret := &v1core.Secret{}

	if OCIsecret.Spec.ArtefactPullSecret.Name == "" || OCIsecret.Spec.ArtefactPullSecret.Namespace == "" {
		// No pull secret specified, will use anonymous access to the registry
		logger.Info("No ArtefactPullSecret specified.")
	} else {
		// Pull secret is specified, fetch it from the cluster
		OCIPullSecretReq := reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      OCIsecret.Spec.ArtefactPullSecret.Name,
				Namespace: OCIsecret.Spec.ArtefactPullSecret.Namespace,
			},
		}
		err = r.Get(ctx, OCIPullSecretReq.NamespacedName, OCIPullSecret)
		if err != nil && apierrors.IsNotFound(err) {
			// The specified pull secret doesn't exist
			logger.Info("ArtefactPullSecret resource not found.")
			return ctrl.Result{}, err
		} else if err != nil {
			// Error fetching the pull secret
			logger.Error(err, "Failed to get ArtefactPullSecret.")
			return ctrl.Result{}, err
		}

		// Extract the Docker config JSON from the pull secret
		for key, value := range OCIPullSecret.Data {
			if key == ".dockerconfigjson" {
				secretData = string(value)
			}
		}

		if secretData == "" {
			// The pull secret doesn't contain Docker config JSON
			logger.Info("No PullSecret Data found.")
			return ctrl.Result{}, nil
		}
	}

	// Step 3: Get the digest of the OCI artifact to detect changes
	// This will be used to determine if the target Secret needs to be updated
	currentDigest := orasclient.GetDigest(OCIsecret.Spec.ArtefactRegistry, OCIsecret.Spec.OrasArtefact, []byte(secretData))

	// Step 4a: Check if the target Secret exists, create it if it doesn't
	TargetSecret := &v1core.Secret{}
	TargetSecretReq := reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      OCIsecret.Spec.TargetSecret.Name,
			Namespace: OCIsecret.Spec.TargetSecret.Namespace,
		},
	}

	// Try to get the target Secret
	err = r.Get(ctx, TargetSecretReq.NamespacedName, TargetSecret)
	if err != nil && apierrors.IsNotFound(err) {
		// Target Secret doesn't exist, create it
		// Initialize with a placeholder revision annotation that will be updated later
		TargetSecret := &v1core.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      OCIsecret.Spec.TargetSecret.Name,
				Namespace: OCIsecret.Spec.TargetSecret.Namespace,
				Annotations: map[string]string{
					"OCISecret.operator.rev": "00000", // Initial placeholder revision
				},
				// Set owner reference to the OCISecret so the Secret is deleted when the OCISecret is deleted
				OwnerReferences: []metav1.OwnerReference{
					{
						APIVersion:         OCIsecret.APIVersion,
						Kind:               OCIsecret.Kind,
						Name:               OCIsecret.Name,
						UID:                OCIsecret.UID,
						Controller:         pointer.Bool(true),
						BlockOwnerDeletion: pointer.Bool(true),
					},
				},
			},
		}

		// Create the target Secret
		err = r.Create(ctx, TargetSecret)
		if err != nil {
			logger.Error(err, "Failed to create TargetSecret.")
		} else {
			logger.Info("Created TargetSecret.")
		}

	} else if err != nil {
		// Error getting the target Secret
		logger.Error(err, "Failed to get TargetSecret.")
		return ctrl.Result{}, err
	}

	// Step 4b: Update the target Secret if needed
	// Refresh our view of the target Secret to ensure we have the latest version
	err = r.Get(ctx, TargetSecretReq.NamespacedName, TargetSecret)
	if err != nil {
		logger.Error(err, "Failed to get TargetSecret.")
		return ctrl.Result{}, err
	}

	// Check if the target Secret needs to be updated:
	// - If the digest has changed (content in the OCI registry has changed)
	// - If the number of files to sync has changed
	if TargetSecret.Annotations["OCISecret.operator.rev"] != currentDigest || len(TargetSecret.Data) != len(OCIsecret.Spec.Sync.Files) {
		logger.Info("TargetSecret needs to be updated.")

		// Download the files from the OCI registry
		content := orasclient.GetFiles(OCIsecret.Spec.ArtefactRegistry, OCIsecret.Spec.OrasArtefact, []byte(secretData))

		// Filter the files based on the OCISecret specification
		if len(OCIsecret.Spec.Sync.Files) > 0 {
			// Only keep files that are specified in the OCISecret.Spec.Sync.Files list
			utils.FilterMapInPlace(content.Files, OCIsecret.Spec.Sync.Files)
		}

		// Update the target Secret with the downloaded files
		TargetSecret.Data = content.Files
		// Update the revision annotation to track the current digest
		TargetSecret.Annotations["OCISecret.operator.rev"] = string(content.Digest)

		// Save the updated target Secret
		err = r.Update(ctx, TargetSecret)
		if err != nil {
			logger.Error(err, "Failed to update TargetSecret.")
			return ctrl.Result{}, err
		} else {
			logger.Info("Updated TargetSecret.")
		}
	}

	// Step 5: Schedule the next reconciliation
	// Requeue after 60 seconds to periodically check for changes in the OCI registry
	return ctrl.Result{RequeueAfter: time.Duration(60) * time.Second}, nil
}

// SetupWithManager sets up the controller with the Manager.
// This method configures the controller to watch OCISecret resources.
//
// The controller-runtime library handles:
// - Starting and stopping the controller
// - Watching for changes to OCISecret resources
// - Calling the Reconcile method when OCISecret resources change
// - Managing the controller's lifecycle
//
// Parameters:
//   - mgr: The controller manager that will manage this controller's lifecycle
//
// Returns:
//   - An error if the controller cannot be set up
func (r *OCISecretReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Watch for changes to OCISecret resources
		For(&ocisyncv1aplha1.OCISecret{}).
		// Complete sets up the controller with the reconciler
		Complete(r)
}
