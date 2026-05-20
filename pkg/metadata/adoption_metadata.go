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

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	ackgenconfig "github.com/aws-controllers-k8s/code-generator/pkg/config"
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
// structured adoption field metadata by analyzing the ReadOne operation's
// input shape (the same logic used by PopulateResourceFromAnnotation).
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

	if crd.IsARNPrimaryKey() {
		res.PrimaryIdentifier = &AdoptionField{
			FieldName: "arn",
			Location:  "metadata",
			Type:      "arn",
		}
		return res
	}

	cfg := crd.Config()

	primaryField, err := crd.GetPrimaryKeyField()
	if err != nil {
		return res
	}

	op := getReadOperation(crd)
	if op == nil && primaryField == nil {
		return res
	}

	if primaryField != nil {
		res.PrimaryIdentifier = &AdoptionField{
			FieldName: primaryField.Names.CamelLower,
			Location:  fieldLocation(crd, primaryField),
			Type:      classifyFieldType(primaryField.Names.Original),
		}
		res.AdditionalKeys = collectAdditionalKeys(cfg, crd, op, primaryField.Names.Original)
		return res
	}

	// Fall back to auto-detection from ReadOne input shape
	if op == nil {
		return res
	}

	primaryCRField, primaryShapeField := findPrimaryIdentifier(cfg, crd, op)
	if primaryShapeField == "ARN" {
		res.PrimaryIdentifier = &AdoptionField{
			FieldName: "arn",
			Location:  "metadata",
			Type:      "arn",
		}
		return res
	}

	if primaryCRField != "" {
		field := findField(crd, primaryCRField)
		if field != nil {
			res.PrimaryIdentifier = &AdoptionField{
				FieldName: field.Names.CamelLower,
				Location:  fieldLocation(crd, field),
				Type:      classifyFieldType(field.Names.Original),
			}
		}
	}

	res.AdditionalKeys = collectAdditionalKeys(cfg, crd, op, primaryCRField)
	return res
}

func getReadOperation(crd *model.CRD) *awssdkmodel.Operation {
	if crd.Ops.ReadOne != nil {
		return crd.Ops.ReadOne
	}
	if crd.Ops.GetAttributes != nil {
		return crd.Ops.GetAttributes
	}
	if crd.Ops.ReadMany != nil {
		return crd.Ops.ReadMany
	}
	return nil
}

func fieldLocation(crd *model.CRD, field *model.Field) string {
	if _, ok := crd.SpecFields[field.Names.Original]; ok {
		return "spec"
	}
	if _, ok := crd.StatusFields[field.Names.Original]; ok {
		return "status"
	}
	// Also check by Camel name since SpecFields is keyed by original name
	for _, f := range crd.SpecFields {
		if f == field {
			return "spec"
		}
	}
	for _, f := range crd.StatusFields {
		if f == field {
			return "status"
		}
	}
	return "spec"
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

func findField(crd *model.CRD, fieldName string) *model.Field {
	if f, ok := crd.SpecFields[fieldName]; ok {
		return f
	}
	if f, ok := crd.StatusFields[fieldName]; ok {
		return f
	}
	return nil
}

// findPrimaryIdentifier replicates the logic from FindPrimaryIdentifierFieldNames
// but returns just the field names without error (best-effort).
func findPrimaryIdentifier(
	cfg *ackgenconfig.Config,
	crd *model.CRD,
	op *awssdkmodel.Operation,
) (crField string, shapeField string) {
	shape := op.InputRef.Shape
	if shape == nil {
		return "", ""
	}

	identifiers := findIdentifiersInShape(crd, shape, op)
	identifiers = append(identifiers, findARNIdentifiersInShape(crd, shape)...)

	switch len(identifiers) {
	case 0:
		return "", ""
	case 1:
		shapeField = identifiers[0]
	default:
		// Multiple identifiers without explicit config — can't determine
		return "", ""
	}

	if crd.IsPrimaryARNField(shapeField) {
		return "", "ARN"
	}

	fieldName := cfg.GetResourceFieldName(
		crd.Names.Original,
		op.ExportedName,
		shapeField,
	)
	return fieldName, shapeField
}

func findIdentifiersInShape(
	crd *model.CRD,
	shape *awssdkmodel.Shape,
	op *awssdkmodel.Operation,
) []string {
	var identifiers []string
	identifierLookup := []string{
		"Id", "ID", "Ids", "IDs",
		crd.Names.Original + "Id",
		crd.Names.Original + "ID",
		crd.Names.Original + "Ids",
		crd.Names.Original + "IDs",
		"Name", "Names",
		crd.Names.Original + "Name",
		crd.Names.Original + "Names",
	}

	opType, _ := model.GetOpTypeAndResourceNameFromOpID(op.ExportedName, crd.Config())
	renames := crd.GetAllRenames(opType)
	for _, memberName := range shape.MemberNames() {
		lookupName := memberName
		if renamedName, found := renames[memberName]; found {
			lookupName = renamedName
		}
		if util.InStrings(lookupName, identifierLookup) {
			identifiers = append(identifiers, lookupName)
		}
	}

	return identifiers
}

func findARNIdentifiersInShape(
	crd *model.CRD,
	shape *awssdkmodel.Shape,
) []string {
	var identifiers []string
	for _, memberName := range shape.MemberNames() {
		if crd.IsPrimaryARNField(memberName) {
			identifiers = append(identifiers, memberName)
		}
	}
	return identifiers
}

// collectAdditionalKeys iterates the ReadOne input shape members and returns
// any required scalar string fields beyond the primary identifier.
func collectAdditionalKeys(
	cfg *ackgenconfig.Config,
	crd *model.CRD,
	op *awssdkmodel.Operation,
	primaryFieldName string,
) []AdoptionField {
	if op == nil {
		return nil
	}
	inputShape := op.InputRef.Shape
	if inputShape == nil {
		return nil
	}

	paginatorFields := []string{"NextToken", "MaxResults"}
	var additional []AdoptionField

	for _, memberName := range inputShape.MemberNames() {
		if util.InStrings(memberName, paginatorFields) {
			continue
		}

		inputShapeRef := inputShape.MemberRefs[memberName]
		inputMemberShape := inputShapeRef.Shape

		if inputMemberShape.Type != "string" &&
			(inputMemberShape.Type != "list" ||
				inputMemberShape.MemberRef.Shape.Type != "string") {
			continue
		}

		if crd.IsSecretField(memberName) {
			continue
		}
		if crd.IsPrimaryARNField(memberName) {
			continue
		}

		fieldName := cfg.GetResourceFieldName(
			crd.Names.Original,
			op.ExportedName,
			memberName,
		)

		if fieldName == primaryFieldName {
			continue
		}

		if !inputShape.IsRequired(memberName) {
			continue
		}

		field := findField(crd, fieldName)
		if field == nil {
			continue
		}

		additional = append(additional, AdoptionField{
			FieldName: field.Names.CamelLower,
			Location:  fieldLocation(crd, field),
			Type:      classifyFieldType(field.Names.Original),
		})
	}

	return additional
}
