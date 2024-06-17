package cloudyazure

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
)

var DefaultTransport policy.Transporter

func is404(err error) bool {
	var respErr *azcore.ResponseError
	if !errors.As(err, &respErr) {
		return false
	}

	if respErr.StatusCode == http.StatusNotFound || bloberror.HasCode(err, bloberror.ResourceNotFound, "ShareNotFound") {
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

func FromStrPointerMap(pointerMap map[string]*string) map[string]string {
	stringMap := make(map[string]string, len(pointerMap))
	for k, v := range pointerMap {
		if v == nil {
			stringMap[k] = ""
		}
		copy := v
		stringMap[k] = *copy
	}
	return stringMap
}

func ToStrPointerMap(stringMap map[string]string) map[string]*string {
	pointerMap := make(map[string]*string, len(stringMap))
	for k, v := range stringMap {
		if v == "" {
			pointerMap[k] = nil
		}
		copy := v
		pointerMap[k] = &copy
	}
	return pointerMap
}
