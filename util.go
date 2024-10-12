package cloudyazure

import (
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
)

var DefaultTransport policy.Transporter

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
