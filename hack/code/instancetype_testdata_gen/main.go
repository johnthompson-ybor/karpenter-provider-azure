/*
Portions Copyright (c) Microsoft Corporation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"

	"github.com/samber/lo"
)

func main() {
	fmt.Println("starting generation of sku data...")
	sub := os.Getenv("SUBSCRIPTION_ID")
	path, region, selectedSkus := os.Args[2], os.Args[3], os.Args[4]
	skus := strings.Split(selectedSkus, ",")
	targetSkus := map[string]struct{}{}
	for _, sku := range skus {
		targetSkus[sku] = struct{}{}
	}
	if sub == "" {
		fmt.Println("SUBSCRIPTION_ID env var is required")
		os.Exit(1)
	}
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		fmt.Printf("failed to create credential: %v", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err != nil {
		fmt.Printf("failed to create client factory: %v", err)
	}

	client, err := armcompute.NewResourceSKUsClient(sub, cred, nil)
	if err != nil {
		panic(err)
	}
	pager := client.NewListPager(&armcompute.ResourceSKUsClientListOptions{
		// filter by location
		Filter: lo.ToPtr(fmt.Sprintf("location eq '%s'", region)),
	})
	skuData := []*armcompute.ResourceSKU{}
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			fmt.Printf("failed to get next page: %v", err)
		}
		for _, sku := range page.Value {
			if _, ok := targetSkus[*sku.Name]; !ok {
				continue
			}
			skuData = append(skuData, sku)
		}
	}
	fmt.Println("Successfully Fetched all the SKUs", len(skuData))
	writeSkuData(skuData, region, path)
	fmt.Println("Successfully Generated all the SKUs")
}

func writeSkuData(ResourceSkus []*armcompute.ResourceSKU, location, path string) {
	src := &bytes.Buffer{}
	fmt.Fprintln(src, "//go:build !ignore_autogenerated")
	license := lo.Must(os.ReadFile("hack/boilerplate.go.txt"))
	fmt.Fprintln(src, string(license))
	fmt.Fprintln(src, "package fake")
	fmt.Fprintln(src, "import (")
	fmt.Fprintln(src, `	"github.com/samber/lo"`)
	fmt.Fprintln(src, `		// nolint SA1019 - deprecated package`)
	fmt.Fprintln(src, `		"github.com/Azure/azure-sdk-for-go/services/compute/mgmt/2022-08-01/compute"`)
	fmt.Fprintln(src, ")")
	now := time.Now().UTC().Format(time.RFC3339)
	fmt.Fprintf(src, "// generated at %s\n\n\n", now)
	fmt.Fprintln(src, "func init() {")
	fmt.Fprintln(src, "// ResourceSkus is a list of selected VM SKUs for a given region")
	fmt.Fprintf(src, "ResourceSkus[%q] = []compute.ResourceSku{\n", location)
	for _, sku := range ResourceSkus {
		fmt.Fprintln(src, "	{")
		fmt.Fprintf(src, "		Name:         lo.ToPtr(%q),\n", lo.FromPtrOr(sku.Name, ""))
		fmt.Fprintf(src, "		Tier:         lo.ToPtr(%q),\n", lo.FromPtrOr(sku.Tier, ""))
		fmt.Fprintf(src, "		Kind:         lo.ToPtr(%q),\n", lo.FromPtrOr(sku.Kind, ""))
		fmt.Fprintf(src, "		Size:         lo.ToPtr(%q),\n", lo.FromPtrOr(sku.Size, ""))
		fmt.Fprintf(src, "		Family:       lo.ToPtr(%q),\n", lo.FromPtrOr(sku.Family, ""))
		fmt.Fprintf(src, "		ResourceType: lo.ToPtr(%q),\n", lo.FromPtrOr(sku.ResourceType, ""))
		fmt.Fprintln(src, "		APIVersions: &[]string{")
		for _, apiVersion := range sku.APIVersions {
			fmt.Fprintf(src, "			lo.ToPtr(%q),\n", lo.FromPtrOr(apiVersion, ""))
		}
		fmt.Fprintln(src, "		},")
		if sku.Capacity != nil {
			fmt.Fprintln(src, "		Capacity: compute.ResourceSkuCapacity{")
			fmt.Fprintf(src, "			Minimum: lo.ToPtr(%d),\n", lo.FromPtrOr(sku.Capacity.Minimum, 0))
			fmt.Fprintf(src, "			Maximum: lo.ToPtr(%d),\n", lo.FromPtrOr(sku.Capacity.Maximum, 0))
			fmt.Fprintf(src, "			Default: lo.ToPtr(%d),\n", lo.FromPtrOr(sku.Capacity.Default, 0))
			fmt.Fprintf(src, "		},")
		}
		fmt.Fprintf(src, "		Costs: &[]compute.ResourceSkuCosts{")
		for _, cost := range sku.Costs {
			fmt.Fprintf(src, "			{MeterID: lo.ToPtr(%f), Quantity: lo.ToPtr(%q), ExtendedUnit: lo.ToPtr(%q)},", lo.FromPtrOr(cost.MeterID, ""), lo.FromPtrOr(cost.Quantity, 0.0), lo.FromPtrOr(cost.ExtendedUnit, ""))
		}
		fmt.Fprintln(src, "		},")
		fmt.Fprintln(src, "		Restrictions: &[]compute.ResourceSkuRestrictions{")
		for _, restriction := range sku.Restrictions {
			fmt.Fprintln(src, "			{")
			fmt.Fprintf(src, "				Type: compute.ResourceSkuRestrictionsType(%q),\n", lo.FromPtrOr(restriction.Type, ""))
			for _, value := range restriction.Values {
				fmt.Fprintf(src, "				Values: &[]string{%q},", lo.FromPtrOr(value, ""))
			}
			fmt.Fprintln(src)
			fmt.Fprintln(src, "				RestrictionInfo: &compute.ResourceSkuRestrictionInfo{")
			fmt.Fprintln(src, "					Locations: &[]string{")
			for _, location := range restriction.RestrictionInfo.Locations {
				fmt.Fprintf(src, "						%q,\n", lo.FromPtr(location))
			}
			fmt.Fprintln(src, "					},")
			fmt.Fprintln(src, "					Zones: &[]string{")
			for _, zone := range restriction.RestrictionInfo.Zones {
				fmt.Fprintf(src, "						%q,\n", lo.FromPtr(zone))
			}
			fmt.Fprintln(src, "					},")
			fmt.Fprintln(src, "				},")
			fmt.Fprintf(src, "				ReasonCode: %q,\n", lo.FromPtrOr(restriction.ReasonCode, ""))
			fmt.Fprintln(src, "			},")
		}
		fmt.Fprintln(src, "		},")
		fmt.Fprintln(src, "		Capabilities: &[]compute.ResourceSkuCapabilities{")
		for _, capability := range sku.Capabilities {
			fmt.Fprintf(src, "			{Name: lo.ToPtr(%q), Value: lo.ToPtr(%q)},\n", *capability.Name, *capability.Value)
		}
		fmt.Fprintln(src, "		},")
		fmt.Fprintf(src, "		Locations: &[]string{%q},\n", location)
		fmt.Fprintf(src, "		LocationInfo: &[]compute.ResourceSkuLocationInfo{")
		for _, locationInfo := range sku.LocationInfo {
			fmt.Fprintf(src, "			{Location: lo.ToPtr(%q),", location)
			fmt.Fprintln(src, "			Zones: &[]string{")
			sort.Slice(locationInfo.Zones, func(i, j int) bool {
				return *locationInfo.Zones[i] < *locationInfo.Zones[j]
			})
			for _, zone := range locationInfo.Zones {
				fmt.Fprintf(src, "				%q,\n", lo.FromPtr(zone))
			}
			fmt.Fprintln(src, "			},")
		}
		fmt.Fprintln(src, "			},")
		fmt.Fprintln(src, "	},")

		fmt.Fprintln(src, "},")
	}

	fmt.Fprintln(src, "}")
	fmt.Fprintln(src, "}")
	fmt.Println("writing file to", path)
	if err := os.WriteFile(path, src.Bytes(), 0600); err != nil {
		fmt.Printf("failed to write file: %v", err)
		panic(err)
	}
}