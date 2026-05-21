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

	ackgenconfig "github.com/aws-controllers-k8s/code-generator/pkg/config"
	"github.com/aws-controllers-k8s/code-generator/pkg/generate/code"
	"github.com/aws-controllers-k8s/code-generator/pkg/model"
	"github.com/aws-controllers-k8s/code-generator/pkg/util"
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
	Note              string          `json:"note,omitempty"`
	PrimaryIdentifier *AdoptionField  `json:"primaryIdentifier,omitempty"`
	AdditionalKeys    []AdoptionField `json:"additionalKeys,omitempty"`
}

// AdoptionField describes a single field used during resource adoption.
type AdoptionField struct {
	FieldName string `json:"fieldName"`
	Location  string `json:"location"` // "spec", "status", or "metadata"
	Type      string `json:"type"`     // "name", "id", "arn"
	Required  bool   `json:"required"`
	Note      string `json:"note,omitempty"`
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
	if err != nil {
		util.Infof("could not determine adoption fields for %s: %v\n", crd.Names.Camel, err)
		return res
	}
	if fields == nil {
		return res
	}

	// Load documentation overrides for adoption
	var adoptionDocs *ackgenconfig.AdoptionDocsConfig
	if docCfg := crd.DocConfig(); docCfg != nil {
		adoptionDocs = docCfg.Adoption
	}

	if adoptionDocs != nil && adoptionDocs.Note != "" {
		res.Note = adoptionDocs.Note
	}

	if fields.IsARNPrimary {
		res.PrimaryIdentifier = &AdoptionField{
			FieldName: "arn",
			Location:  "metadata",
			Type:      "arn",
			Required:  true,
		}
		return res
	}

	for _, id := range fields.Identifiers {
		fieldDoc := getFieldDocOverride(adoptionDocs, id.FieldName)

		if fieldDoc != nil && fieldDoc.Hidden {
			continue
		}

		required := id.Required
		if fieldDoc != nil && fieldDoc.Required != nil {
			required = *fieldDoc.Required
		}

		note := ""
		if fieldDoc != nil {
			note = fieldDoc.Note
		}

		af := AdoptionField{
			FieldName: id.FieldName,
			Location:  locationString(id.InSpec),
			Type:      classifyFieldType(id.Field.Names.Original),
			Required:  required,
			Note:      note,
		}
		if id.IsPrimary {
			res.PrimaryIdentifier = &af
		} else {
			res.AdditionalKeys = append(res.AdditionalKeys, af)
		}
	}

	return res
}

func getFieldDocOverride(
	adoptionDocs *ackgenconfig.AdoptionDocsConfig,
	fieldName string,
) *ackgenconfig.AdoptionFieldDocsConfig {
	if adoptionDocs == nil || adoptionDocs.Fields == nil {
		return nil
	}
	return adoptionDocs.Fields[fieldName]
}

func locationString(inSpec bool) string {
	if inSpec {
		return "spec"
	}
	return "status"
}

func classifyFieldType(originalName string) string {
	lower := strings.ToLower(originalName)
	if strings.HasSuffix(lower, "arn") || strings.Contains(lower, "arn_") {
		return "arn"
	}
	if strings.HasSuffix(lower, "id") || strings.HasSuffix(lower, "ids") || strings.Contains(lower, "id_") {
		return "id"
	}
	return "name"
}
