package cloudyazure

import (
	"context"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/arm"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/cloud"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization"
	"github.com/appliedres/cloudy"
)

type AzureVirtualDesktopConfig struct {
	AzureCredentials
	subscription string
}

type AzureVirtualDesktop struct {
	config       AzureVirtualDesktopConfig
	workspaces   *armdesktopvirtualization.WorkspacesClient
	hostpools    *armdesktopvirtualization.HostPoolsClient
	sessionhosts *armdesktopvirtualization.SessionHostsClient
}

func NewAzureVirtualDesktop(ctx context.Context, config AzureVirtualDesktopConfig) (*AzureVirtualDesktop, error) {
	cred, err := GetAzureClientSecretCredential(config.AzureCredentials)
	if err != nil {
		return nil, cloudy.Error(ctx, "Authentication failure: %+v", err)
	}

	options := arm.ClientOptions{
		ClientOptions: azcore.ClientOptions{
			Cloud: cloud.AzureGovernment,
		},
	}
	workspaces, err := armdesktopvirtualization.NewWorkspacesClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewWorkspacesClient failure: %+v", err)
	}

	hostpools, err := armdesktopvirtualization.NewHostPoolsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewHostPoolsClient failure: %+v", err)
	}

	sessionhosts, err := armdesktopvirtualization.NewSessionHostsClient(config.subscription, cred, &options)
	if err != nil {
		return nil, cloudy.Error(ctx, "NewSessionHostsClient failure: %+v", err)
	}

	return &AzureVirtualDesktop{
		config:       config,
		workspaces:   workspaces,
		hostpools:    hostpools,
		sessionhosts: sessionhosts,
	}, nil
}

func (avd *AzureVirtualDesktop) ListHostPools(ctx context.Context, rg string) ([]*armdesktopvirtualization.HostPool, error) {
	pager := avd.hostpools.NewListByResourceGroupPager(rg, &armdesktopvirtualization.HostPoolsClientListByResourceGroupOptions{})
	var all []*armdesktopvirtualization.HostPool
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.HostPoolList.Value...)
	}
	return all, nil
}

func (avd *AzureVirtualDesktop) ListSessionHosts(ctx context.Context, rg string, hostPool string) ([]*armdesktopvirtualization.SessionHost, error) {
	pager := avd.sessionhosts.NewListPager(rg, hostPool, nil)
	var all []*armdesktopvirtualization.SessionHost
	for {
		if !pager.More() {
			break
		}
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		all = append(all, resp.Value...)
	}
	return all, nil
}

func (avd *AzureVirtualDesktop) FindFirstAvailableHostPool(ctx context.Context, rg string, upn string) (*armdesktopvirtualization.HostPool, error) {
	// Get all the host pools
	all, err := avd.ListHostPools(ctx, rg)
	if err != nil {
		return nil, err
	}

	for _, hostpool := range all {
		// List all the sessions for a given host pool
		sessions, err := avd.ListSessionHosts(ctx, rg, *hostpool.Name)
		if err != nil {
			return nil, err
		}

		foundUser := false
		for _, session := range sessions {
			assigned := session.Properties.AssignedUser
			if assigned != nil && *assigned == upn {
				foundUser = true
				break
			}
		}

		if !foundUser {
			return hostpool, nil
		}
	}

	return nil, nil
}

func (avd *AzureVirtualDesktop) RetrieveRegistrationToken(ctx context.Context, rg string, hpname string) (*string, error) {

	// avd.hostpools.RetrieveRegistrationToken returns nil if registration token doesn't exist or is expired
	tokenresponse, err := avd.hostpools.RetrieveRegistrationToken(ctx, rg, hpname, nil)

	if tokenresponse.Token == nil {
		// no go function to create/replace a registration key in armdesktopvirtualization
		return nil, cloudy.Error(ctx, "RetrieveRegistrationToken failure: %+v", err)
	}

	return tokenresponse.Token, err
}
