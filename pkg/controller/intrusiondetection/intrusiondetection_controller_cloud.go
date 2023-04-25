// Copyright (c) 2022 Tigera, Inc. All rights reserved.

package intrusiondetection

import (
	"context"
	"fmt"

	operatorv1 "github.com/tigera/operator/api/v1"
	"github.com/tigera/operator/pkg/common"

	"github.com/go-logr/logr"

	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/tigera/operator/pkg/controller/utils"
	"github.com/tigera/operator/pkg/render"
	rcimageassurance "github.com/tigera/operator/pkg/render/common/imageassurance"
	iarender "github.com/tigera/operator/pkg/render/imageassurance"
)

const (
	ImageAssuranceAPIAccessResourceName = "tigera-image-assurance-intrusion-detection-controller-api-access"
)

func addCloudWatch(c controller.Controller) error {
	if err := utils.AddImageAssuranceWatch(c, render.IntrusionDetectionNamespace); err != nil {
		return err
	}

	if err := utils.AddClusterRoleWatch(c, render.IntrusionDetectionControllerImageAssuranceAPIClusterRoleName); err != nil {
		return err
	}

	if err := utils.AddSecretsWatch(c, ImageAssuranceAPIAccessResourceName, common.OperatorNamespace()); err != nil {
		return err
	}

	return nil
}

// handleCloudResources returns managerCloudResources.
// It returns a non-nil reconcile.Result when it's waiting for resources to be available
func (r *ReconcileIntrusionDetection) handleCloudResources(ctx context.Context, reqLogger logr.Logger) (render.IntrusionDetectionCloudResources, *reconcile.Result, error) {
	idcr := render.IntrusionDetectionCloudResources{}
	if _, err := utils.GetImageAssurance(ctx, r.client); err != nil {
		if errors.IsNotFound(err) {
			reqLogger.Info("Image Assurance CR is not found, continuing without enabling Image Assurance")
			return idcr, nil, nil
		}
		reqLogger.Error(err, "failed to check for Image Assurance existence")
		r.status.SetDegraded(operatorv1.ResourceReadError, "failed to check for Image Assurance existence", err, reqLogger)
		return idcr, nil, err
	}

	// get tls secret for image assurance api communication
	secret, err := utils.GetImageAssuranceTLSSecret(r.client)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("failed to retrieve secret %s", iarender.APICertSecretName))
		r.status.SetDegraded(operatorv1.ResourceReadError, fmt.Sprintf("Failed to retrieve secret %s", iarender.APICertSecretName), err, reqLogger)
		return idcr, nil, err
	} else if secret == nil {
		reqLogger.Info(fmt.Sprintf("waiting for secret '%s' to become available", iarender.APICertSecretName))
		r.status.SetDegraded(operatorv1.ResourceNotReady, fmt.Sprintf("waiting for secret '%s' to become available", iarender.APICertSecretName), nil, reqLogger)
		return idcr, &reconcile.Result{}, nil
	}

	// get image assurance configuration config map
	cm, err := utils.GetImageAssuranceConfigurationConfigMap(r.client)
	if err != nil {
		reqLogger.Error(err, fmt.Sprintf("failed to retrieve configmap: %s", rcimageassurance.ConfigurationConfigMapName))
		r.status.SetDegraded(operatorv1.ResourceReadError, fmt.Sprintf("failed to retrieve configmap: %s", rcimageassurance.ConfigurationConfigMapName), err, reqLogger)
		return idcr, nil, err
	}

	iaToken, err := utils.GetImageAssuranceAPIAccessToken(r.client, ImageAssuranceAPIAccessResourceName)
	if err != nil {
		reqLogger.Error(err, err.Error())
		r.status.SetDegraded(operatorv1.ResourceReadError, "Error in retrieving image assurance API access token", err, reqLogger)
		return idcr, nil, err
	}

	if iaToken == nil {
		reqLogger.Info("Waiting for image assurance API access token to be available")
		r.status.SetDegraded(operatorv1.ResourceReadError, "Waiting for image assurance API access token to be available", nil, reqLogger)
		return idcr, &reconcile.Result{}, nil
	}

	idcr.ImageAssuranceResources = &rcimageassurance.Resources{
		ConfigurationConfigMap: cm,
		TLSSecret:              secret,
		ImageAssuranceToken:    iaToken,
	}
	reqLogger.Info("Successfully processed resources for Image Assurance")

	return idcr, nil, nil
}