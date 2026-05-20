// Copyright Amazon.com Inc. or its affiliates. All Rights Reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License"). You may
// not use this file except in compliance with the License. A copy of the
// License is located at
//
//     http://aws.amazon.com/apache2.0/
//
// or in the "license" file accompanying this file. This file is distributed
// on an "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either
// express or implied. See the License for the specific language governing
// permissions and limitations under the License.

package metadata

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws-controllers-k8s/code-generator/pkg/generate/code"
	"github.com/aws-controllers-k8s/code-generator/pkg/model"
)

// AdoptionMetadata is the top-level structure written to adoption-metadata.json.
type AdoptionMetadata struct {
	Service   string             `json:"service"`
	Resources []AdoptionResource `json:"resources"`
}

// AdoptionResource describes the adoption fields for a single CRD.
type AdoptionResource struct {
	Kind              string          `json:"kind"`
	Adoptable         bool            `json:"adoptable"`
	PrimaryIdentifier *AdoptionField  `json:"primaryIdentifier,omitempty"`
	AdditionalKeys    []AdoptionField `json:"additionalKeys,omitempty"`
}

// AdoptionField describes a single field used during resource adoption.
type AdoptionField struct {
	FieldName string `json:"fieldName"`
	Location  string `json:"location"` // "spec", "status", or "metadata"
	Type      string `json:"type"`     // "name", "id", "arn"
}

// GenerateAdoptionMetadata inspects all CRDs in the model and produces
// structured adoption field metadata using the shared GetAdoptionFields logic.
func GenerateAdoptionMetadata(
	serviceName string,
	crds []*model.CRD,
) *AdoptionMetadata {
	meta := &AdoptionMetadata{
		Service: serviceName,
	}

	for _, crd := range crds {
		resource := buildAdoptionResource(crd)
		meta.Resources = append(meta.Resources, resource)
	}

	return meta
}

// WriteAdoptionMetadata marshals the adoption metadata to JSON and writes it
// to the given path.
func WriteAdoptionMetadata(path string, meta *AdoptionMetadata) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("creating adoption metadata file: %w", err)
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode(meta)
}

func buildAdoptionResource(crd *model.CRD) AdoptionResource {
	res := AdoptionResource{
		Kind:      crd.Names.Camel,
		Adoptable: crd.IsAdoptable(),
	}

	if !res.Adoptable {
		return res
	}

	fields, err := code.GetAdoptionFields(crd.Config(), crd)
	if err != nil || fields == nil {
		return res
	}

	if fields.IsARNPrimary {
		res.PrimaryIdentifier = &AdoptionField{
			FieldName: "arn",
			Location:  "metadata",
			Type:      "arn",
		}
		return res
	}

	for _, id := range fields.Identifiers {
		af := AdoptionField{
			FieldName: id.FieldName,
			Location:  locationString(id.InSpec),
			Type:      classifyFieldType(id.Field.Names.Original),
		}
		if id.IsPrimary {
			res.PrimaryIdentifier = &af
		} else if id.Required {
			res.AdditionalKeys = append(res.AdditionalKeys, af)
		}
	}

	return res
}

func locationString(inSpec bool) string {
	if inSpec {
		return "spec"
	}
	return "status"
}

func classifyFieldType(originalName string) string {
	lower := strings.ToLower(originalName)
	if strings.Contains(lower, "arn") {
		return "arn"
	}
	if strings.Contains(lower, "id") {
		return "id"
	}
	return "name"
}
