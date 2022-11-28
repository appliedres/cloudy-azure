package cloudyazure

import (
	"errors"
	"strings"

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

func sanitizeName(name string) string {
	name = strings.ReplaceAll(name, ".", "-")
	name = strings.ReplaceAll(name, "'", "-")
	name = strings.ReplaceAll(name, "_", "-")

	return strings.ToLower(name)
}

func FromStrPointerMap(in map[string]*string) map[string]string {
	out := make(map[string]string)
	for k, v := range in {
		if v != nil {
			out[k] = *v
		} else {
			out[k] = ""
		}
	}
	return out
}

func ToStrPointerMap(in map[string]string) map[string]*string {
	out := make(map[string]*string)
	for k, v := range in {
		if v != "" {
			out[k] = &v
		} else {
			out[k] = nil
		}
	}
	return out
}
