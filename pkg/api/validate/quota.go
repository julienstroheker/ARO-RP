package validate

// Copyright (c) Microsoft Corporation.
// Licensed under the Apache License 2.0.

import (
	"context"
	"fmt"
	"net/http"

	"github.com/Azure/ARO-RP/pkg/api"
	"github.com/Azure/ARO-RP/pkg/util/azureclient/mgmt/compute"
)

func addRequiredResources(requiredResources map[string]int, vmSize api.VMSize, count int) error {
	requiredResources["virtualMachines"] += count
	requiredResources["PremiumDiskCount"] += count
	switch vmSize {
	case api.VMSizeStandardD2sV3:
		requiredResources["standardDSv3Family"] += (count * 2)
		requiredResources["cores"] += (count * 2)
	case api.VMSizeStandardD4sV3:
		requiredResources["standardDSv3Family"] += (count * 4)
		requiredResources["cores"] += (count * 4)
	case api.VMSizeStandardD8sV3:
		requiredResources["standardDSv3Family"] += (count * 8)
		requiredResources["cores"] += (count * 8)
	default:
		//will only happen if pkg/api verification allows new VMSizes
		return fmt.Errorf("unexpected node VMSize %s", vmSize)
	}
	return nil
}

// validateQuotas checks usage quotas vs. resources required by cluster before cluster creation
func (dv *openShiftClusterDynamicValidator) validateQuotas(ctx context.Context, oc *api.OpenShiftCluster, uc compute.UsageClient) error {
	requiredResources := map[string]int{}
	addRequiredResources(requiredResources, oc.Properties.MasterProfile.VMSize, 3)
	//worker node resource calculation
	for _, w := range oc.Properties.WorkerProfiles {
		addRequiredResources(requiredResources, w.VMSize, w.Count)
	}

	usages, err := uc.List(ctx, oc.Location)
	if err != nil {
		return err
	}
	//check requirements vs. usage

	// we're only checking the limits returned by the Usage API and ignoring usage limits missing from the results
	// rationale:
	// 1. if the Usage API doesn't send a limit because a resource is no longer limited, RP will continue cluster creation without impact
	// 2. if the Usage API doesn't send a limit that is still enforced, cluster creation will fail on the backend and we will get an error in the RP logs
	for _, usage := range usages {
		required, present := requiredResources[*usage.Name.Value]
		if present && int64(required) > (*usage.Limit-int64(*usage.CurrentValue)) {
			return api.NewCloudError(http.StatusBadRequest, api.CloudErrorCodeResourceQuotaExceeded, "", "Resource quota of %s exceeded. Maximum allowed: %d, Current in use: %d, Additional requested: %d.", *usage.Name.Value, *usage.Limit, *usage.CurrentValue, required)
		}
	}
	return nil
}
