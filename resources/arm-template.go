package resources

import (
	"context"
	"encoding/json"
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

func (arm *ArmManager) ExecuteArmTemplate(ctx context.Context, name string, templateData []byte, paramsData []byte) error {
	var template map[string]interface{}
	var params map[string]interface{}

	log := logging.GetLogger(ctx)

	err := json.Unmarshal(templateData, &template)
	if err != nil {
		log.ErrorContext(ctx, "Failed to unmarshall arm template data", logging.WithError(err))
		return err
	}

	err = json.Unmarshal(paramsData, &params)
	if err != nil {
		log.ErrorContext(ctx, "Failed to unmarshall arm template data", logging.WithError(err))
		return err
	}

	timeout, err := time.ParseDuration(arm.config.PollingTimeoutDuration + "m")
	if err != nil {
		log.ErrorContext(ctx, "Failed to convert polling timeout to int", logging.WithError(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	poller, err := arm.client.BeginCreateOrUpdate(
		ctx,
		arm.config.ResourceGroup,
		name,
		armresources.Deployment{
			Location: &arm.config.Location,
			Properties: &armresources.DeploymentProperties{
				Mode:       toPtr(armresources.DeploymentModeIncremental),
				Template:   template,
				Parameters: params,
			},
		},
		nil,
	)
	if err != nil {
		log.ErrorContext(ctx, "Failed to create deployment", logging.WithError(err))
		return err
	}

	resp, err := poller.PollUntilDone(ctx, nil)
	if err != nil {
		log.ErrorContext(ctx, "Failed to unmarshall arm template data", logging.WithError(err))
		return err
	}

	log.InfoContext(ctx, fmt.Sprintf("ARM Deployment %v %v completed successfully", name, resp.ID))
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
