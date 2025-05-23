package avd

import (
	"context"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/desktopvirtualization/armdesktopvirtualization/v2"
	"github.com/appliedres/cloudy"
)

func (avd *AzureVirtualDesktopManager) getUserSessionId(ctx context.Context, hostPoolName string, sessionHost string, upn string) (*string, error) {
	pager := avd.userSessionsClient.NewListPager(avd.Credentials.ResourceGroup, hostPoolName, sessionHost, nil)
	var all []*armdesktopvirtualization.UserSession
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

	for _, userSession := range all {
		if *userSession.Properties.UserPrincipalName == upn {
			temp := *userSession.Name
			lastInd := strings.LastIndex(temp, "/")
			sessionId := temp[lastInd+1:]
			return &sessionId, nil
		}
	}

	return nil, nil
}

func (avd *AzureVirtualDesktopManager) DeleteUserSession(ctx context.Context, hostPoolName string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, hostPoolName, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Delete(ctx, avd.Credentials.ResourceGroup, hostPoolName, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session delete failed): %+v", err)
	}
	_ = res

	return nil
}

func (avd *AzureVirtualDesktopManager) DisconnectUserSession(ctx context.Context, hostPoolName string, sessionHost string, upn string) error {
	sessionId, err := avd.getUserSessionId(ctx, hostPoolName, sessionHost, upn)
	if err != nil {
		return cloudy.Error(ctx, "DisconnectUserSession failure (no user session): %+v", err)
	}

	res, err := avd.userSessionsClient.Disconnect(ctx, avd.Credentials.ResourceGroup, hostPoolName, sessionHost, *sessionId, nil)
	if err != nil {
		return cloudy.Error(ctx, "UnassignSessionHost failure (user session disconnect failed ): %+v", err)
	}
	_ = res

	return nil
}
