package cloudyazure

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
)

func is404(err error) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == 404 {
		return true
	}
	return false
}
