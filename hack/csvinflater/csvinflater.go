// csvinflater adds additional information to the ClusterServiceVersion (CSV) resource. The CSV is the package format
// for the Operator Framework.
//
// This program collects information from our operator sources and writes a kustomize patch for the CSV generated by
// operator-sdk. In particular, it:
// * Annotates all CustomResourceDefinitions with the type of k8s resource it controls.
// * Annotates the CSV with all the kubernetes native resource version it expects.
package main

import (
	"embed"
	"fmt"
	"log"
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/krusty"
	"sigs.k8s.io/kustomize/api/resmap"
	kusttypes "sigs.k8s.io/kustomize/api/types"
	"sigs.k8s.io/kustomize/kyaml/resid"
	"sigs.k8s.io/yaml"

	piraeusv1 "github.com/piraeusdatastore/piraeus-operator/v2/api/v1"
	"github.com/piraeusdatastore/piraeus-operator/v2/pkg/resources"
	"github.com/piraeusdatastore/piraeus-operator/v2/pkg/resources/cluster"
	"github.com/piraeusdatastore/piraeus-operator/v2/pkg/resources/satellite"
	"github.com/piraeusdatastore/piraeus-operator/v2/pkg/utils"
)

type apiResource struct {
	Kind    string `json:"kind"`
	Version string `json:"version"`
	Name    string `json:"name"`
}

func main() {
	clusterRes, err := CombineAllEmbededResources(&cluster.Resources)
	if err != nil {
		log.Fatalf("failed to load cluster resources: %s", err)
	}

	satelliteRes, err := CombineAllEmbededResources(&satellite.Resources)
	if err != nil {
		log.Fatalf("failed to load satellite resources: %s", err)
	}

	patch := []utils.JsonPatch{
		{
			Op:    utils.Add,
			Path:  "/spec/nativeAPIs",
			Value: GetNativeAPI(append(clusterRes.AllIds(), satelliteRes.AllIds()...)...),
		},
		{
			Op:    utils.Test,
			Path:  "/spec/customresourcedefinitions/owned/0/kind",
			Value: "LinstorCluster",
		},
		{
			Op:    utils.Add,
			Path:  "/spec/customresourcedefinitions/owned/0/resources",
			Value: GetApiResources(clusterRes.AllIds()...),
		},
		{
			Op:    utils.Test,
			Path:  "/spec/customresourcedefinitions/owned/1/kind",
			Value: "LinstorSatelliteConfiguration",
		},
		{
			Op:   utils.Add,
			Path: "/spec/customresourcedefinitions/owned/1/resources",
			Value: []apiResource{
				// Technically this is a lie but scorecard is not happy otherwise. The Configuration itself does not
				// create any kind of resource, only in combination with a LinstorSatellite.
				{Version: piraeusv1.GroupVersion.Version, Kind: "LinstorSatellite"},
			},
		},
		{
			Op:    utils.Test,
			Path:  "/spec/customresourcedefinitions/owned/2/kind",
			Value: "LinstorSatellite",
		},
		{
			Op:    utils.Add,
			Path:  "/spec/customresourcedefinitions/owned/2/resources",
			Value: GetApiResources(satelliteRes.AllIds()...),
		},
	}

	bytes, err := yaml.Marshal(patch)
	if err != nil {
		log.Fatalf("failed to marshal patch: %s", err)
	}

	fmt.Println("# File generated by csvinflater. DO NOT EDIT.")
	fmt.Print(string(bytes))
}

func CombineAllEmbededResources(fs *embed.FS) (resmap.ResMap, error) {
	kustomizer, err := resources.NewKustomizer(fs, krusty.MakeDefaultOptions())
	if err != nil {
		return nil, fmt.Errorf("failed to create kustomizer: %w", err)
	}

	subDirs, err := fs.ReadDir(".")
	if err != nil {
		return nil, fmt.Errorf("failed to list embedded directories: %w", err)
	}

	var subDirNames []string
	for i := range subDirs {
		if subDirs[i].Name() == "patches" {
			continue
		}

		subDirNames = append(subDirNames, subDirs[i].Name())
	}

	return kustomizer.Kustomize(&kusttypes.Kustomization{
		Resources: subDirNames,
	})
}

func GetApiResources(rs ...resid.ResId) []apiResource {
	allGvk := make(map[string]struct{})
	for _, r := range rs {
		allGvk[r.Gvk.String()] = struct{}{}
	}

	result := make([]apiResource, 0, len(allGvk))
	for k := range allGvk {
		gvk := resid.GvkFromString(k)
		result = append(result, apiResource{
			Kind:    gvk.Kind,
			Version: gvk.Version,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Version == result[j].Version {
			return result[i].Kind < result[j].Kind
		}
		return result[i].Version < result[j].Version
	})

	return result
}

func GetNativeAPI(rs ...resid.ResId) []metav1.GroupVersionKind {
	allGvk := make(map[string]struct{})
	for _, r := range rs {
		if r.Group != piraeusv1.GroupVersion.Group {
			allGvk[r.Gvk.String()] = struct{}{}
		}
	}

	result := make([]metav1.GroupVersionKind, 0, len(allGvk))
	for k := range allGvk {
		gvk := resid.GvkFromString(k)
		result = append(result, metav1.GroupVersionKind{
			Group:   gvk.Group,
			Version: gvk.Version,
			Kind:    gvk.Kind,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].Group != result[j].Group {
			return result[i].Group < result[j].Group
		}
		if result[i].Version != result[j].Version {
			return result[i].Version < result[j].Version
		}
		return result[i].Kind < result[j].Kind
	})

	return result
}