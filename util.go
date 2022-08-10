package cloudyazure

import (
	"errors"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

func is404(err error) bool {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == 404 {
		return true
	}
	var stErr *azblob.StorageError

	if errors.As(err, &stErr) && stErr.StatusCode() == 404 {
		return true
	}
	return false
}
