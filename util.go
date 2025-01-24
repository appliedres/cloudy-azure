package cloudyazure

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/runtime"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob/bloberror"
	"github.com/appliedres/cloudy/logging"
	"github.com/pkg/errors"
)

var DefaultTransport policy.Transporter

func SanitizeName(name string) string {
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

func ToResponseError(err error) *azcore.ResponseError {
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) {
		return respErr
	}

	return nil
}

func Is404(err error) bool {
	respErr := ToResponseError(err)

	if respErr != nil && respErr.StatusCode == http.StatusNotFound || bloberror.HasCode(err, bloberror.ResourceNotFound, "ShareNotFound") {
		return true
	}

	return false
}

func PollWrapper[T any](ctx context.Context, poller *runtime.Poller[T], pollerType string) (*T, error) {
	log := logging.GetLogger(ctx)

	ticker := time.NewTicker(5 * time.Second)
	startTime := time.Now()
	defer ticker.Stop()
	defer func() {
		log.InfoContext(ctx, fmt.Sprintf("%s complete (elapsed: %s)", pollerType,
			fmt.Sprintf("%.0f seconds", time.Since(startTime).Seconds())))
	}()

	for {
		select {
		case <-ticker.C:
			log.InfoContext(ctx, fmt.Sprintf("Waiting for %s to complete (elapsed: %s)",
				pollerType, fmt.Sprintf("%.0f seconds", time.Since(startTime).Seconds())))
		default:
			_, err := poller.Poll(ctx)
			if err != nil {
				return nil, errors.Wrapf(err, "pollWrapper: %s (Poll)", pollerType)
			}
			if poller.Done() {
				response, err := poller.Result(ctx)

				if err != nil {
					return nil, errors.Wrapf(err, "pollWrapper: %s (Result)", pollerType)
				}

				return &response, nil
			}
		}
	}
}
