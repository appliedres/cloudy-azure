package resources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armresources"
	cloudyazure "github.com/appliedres/cloudy-azure"
	"github.com/appliedres/cloudy/logging"
)

type ArmConfig struct {
	SubscriptionID         string
	Location               string
	ResourceGroup          string
	PollingTimeoutDuration string
}

type ArmManager struct {
	token  azcore.TokenCredential
	config *ArmConfig
	client *armresources.DeploymentsClient
}

func NewArmManager(ctx context.Context, config *ArmConfig, creds *cloudyazure.AzureCredentials) (*ArmManager, error) {
	log := logging.GetLogger(ctx)

	cfg := setArmConfig(config, creds)

	cred, err := cloudyazure.NewAzureCredentials(creds)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create azure token", logging.WithError(err))
		return nil, err
	}

	clientOpts := &arm.ClientOptions{
		ClientOptions: policy.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}

	armClient, err := armresources.NewDeploymentsClient(cfg.SubscriptionID, cred, clientOpts)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create deployment client", logging.WithError(err))
		return nil, err
	}

	return &ArmManager{
		token:  cred,
		config: cfg,
		client: armClient,
	}, nil
}

func (arm *ArmManager) ValidateArmTemplate(ctx context.Context, name string, scope string, templateData []byte, paramsData []byte) (map[string]interface{}, map[string]interface{}, error) {
	log := logging.GetLogger(ctx)

	// Unmarshal ARM template
	var template map[string]interface{}
	if err := json.Unmarshal(templateData, &template); err != nil {
		log.ErrorContext(ctx, "Failed to unmarshal ARM template", logging.WithError(err))
		return nil, nil, err
	}

	// Unmarshal and extract the parameters object
	var fullParams map[string]interface{}
	if err := json.Unmarshal(paramsData, &fullParams); err != nil {
		log.ErrorContext(ctx, "Failed to unmarshal ARM parameters", logging.WithError(err))
		return nil, nil, err
	}

	paramsRaw, ok := fullParams["parameters"]
	if !ok {
		err := fmt.Errorf("missing 'parameters' key in parameters JSON")
		log.ErrorContext(ctx, "Invalid ARM parameters format", logging.WithError(err))
		return nil, nil, err
	}

	params, ok := paramsRaw.(map[string]interface{})
	if !ok {
		err := fmt.Errorf("'parameters' is not a valid object")
		log.ErrorContext(ctx, "Invalid ARM parameters format", logging.WithError(err))
		return nil, nil, err
	}

	deployment := armresources.Deployment{
		Properties: &armresources.DeploymentProperties{
			Mode:       toPtr(armresources.DeploymentModeIncremental),
			Template:   template,
			Parameters: params,
		},
	}

	if scope != "resourceGroup" {
		deployment.Location = toPtr(arm.config.Location)
	}

	// Parse timeout and preserve caller context
	timeout, err := time.ParseDuration(arm.config.PollingTimeoutDuration + "m")
	if err != nil {
		log.ErrorContext(ctx, "Failed to parse polling timeout duration", logging.WithError(err))
		return nil, nil, err
	}

	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	// Validate the ARM template
	poller, err := arm.client.BeginValidate(ctx, arm.config.ResourceGroup, name, deployment, nil)
	if err != nil {
		log.ErrorContext(ctx, "ARM template validation failed", logging.WithError(err))
		return nil, nil, err
	}

	// Wait for deployment completion
	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		log.ErrorContext(ctx, "ARM validation failed during polling", logging.WithError(err))

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			log.ErrorContext(ctx, fmt.Sprintf("StatusCode: %d, ErrorCode: %s, Message: %s", respErr.StatusCode, respErr.ErrorCode, respErr.Error()))
		}

		return template, params, err
	}

	// Log final provisioning state
	if resp.Properties != nil {
		state := "unknown"
		if resp.Properties.ProvisioningState != nil {
			state = string(*resp.Properties.ProvisioningState)
		}
		log.InfoContext(ctx, fmt.Sprintf("ARM Validation '%s' completed with state: %s", name, state))
	}

	return template, params, nil
}

func (arm *ArmManager) DeployValidArmTemplate(ctx context.Context, name string, scope string, templateData map[string]interface{}, paramsData map[string]interface{}) error {
	return arm.deploy(ctx, name, scope, templateData, paramsData)
}

func (arm *ArmManager) DeployArmTemplate(ctx context.Context, name string, scope string, templateData []byte, paramsData []byte) error {
	log := logging.GetLogger(ctx)

	template, params, err := arm.ValidateArmTemplate(ctx, name, scope, templateData, paramsData)
	if err != nil {
		log.ErrorContext(ctx, "ARM template validation failed", logging.WithError(err))
		return err
	}

	return arm.deploy(ctx, name, scope, template, params)
}

func (arm *ArmManager) deploy(ctx context.Context, name string, scope string, template map[string]interface{}, params map[string]interface{}) error {
	log := logging.GetLogger(ctx)

	// Parse timeout in minutes and preserve caller context
	timeout, err := time.ParseDuration(arm.config.PollingTimeoutDuration + "m")
	if err != nil {
		timeout = 30 * time.Minute
		log.DebugContext(ctx, "Failed to parse polling timeout duration, using default", logging.WithError(err))
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	log.InfoContext(ctx, fmt.Sprintf("Starting ARM deployment '%s' with timeout: %.0f minutes", name, timeout.Minutes()))
	log.InfoContext(ctx, fmt.Sprintf("Using deployment location: %s", arm.config.Location))

	// Prepare deployment payload
	deployment := armresources.Deployment{
		Properties: &armresources.DeploymentProperties{
			Mode:       toPtr(armresources.DeploymentModeIncremental),
			Template:   template,
			Parameters: params,
		},
	}

	if scope != "resourceGroup" {
		deployment.Location = toPtr(arm.config.Location)
	}

	// Begin deployment
	poller, err := arm.client.BeginCreateOrUpdate(ctx, arm.config.ResourceGroup, name, deployment, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to start ARM deployment", logging.WithError(err))
		return err
	}

	// Wait for deployment completion
	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		log.ErrorContext(ctx, "ARM deployment failed during polling", logging.WithError(err))

		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) {
			log.ErrorContext(ctx, fmt.Sprintf("StatusCode: %d, ErrorCode: %s, Message: %s", respErr.StatusCode, respErr.ErrorCode, respErr.Error()))
		}

		return err
	}

	// Log final provisioning state and deployment ID
	if resp.Properties != nil {
		state := "unknown"
		if resp.Properties.ProvisioningState != nil {
			state = string(*resp.Properties.ProvisioningState)
		}
		log.InfoContext(ctx, fmt.Sprintf("ARM Deployment '%s' completed with state: %s", name, state))
	}
	if resp.ID != nil {
		log.InfoContext(ctx, fmt.Sprintf("Deployment ID: %s", *resp.ID))
	}

	return nil
}

func setArmConfig(cfg *ArmConfig, creds *cloudyazure.AzureCredentials) *ArmConfig {
	if cfg.Location == "" {
		cfg.Location = creds.Region
	}
	if cfg.PollingTimeoutDuration == "" {
		cfg.PollingTimeoutDuration = "30"
	}
	if cfg.ResourceGroup == "" {
		cfg.ResourceGroup = creds.ResourceGroup
	}
	if cfg.SubscriptionID == "" {
		cfg.SubscriptionID = creds.SubscriptionID
	}

	return cfg
}

func toPtr[T any](v T) *T {
	return &v
}
