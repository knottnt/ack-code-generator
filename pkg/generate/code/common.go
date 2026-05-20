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

package code

import (
	"fmt"
	"strings"

	awssdkmodel "github.com/aws-controllers-k8s/code-generator/pkg/api"
	"github.com/gertd/go-pluralize"

	ackgenconfig "github.com/aws-controllers-k8s/code-generator/pkg/config"
	"github.com/aws-controllers-k8s/code-generator/pkg/model"
	"github.com/aws-controllers-k8s/code-generator/pkg/util"
)

var (
	PrimaryIdentifierARNOverride = "ARN"
)

// FindIdentifiersInShape returns the identifier fields of a given shape which
// can be singular or plural.
func FindIdentifiersInShape(
	r *model.CRD,
	shape *awssdkmodel.Shape,
	op *awssdkmodel.Operation,
) []string {
	var identifiers []string
	if r == nil || shape == nil {
		return identifiers
	}

	identifierLookup := []string{
		"Id",
		"ID",
		"Ids",
		"IDs",
		r.Names.Original + "Id",
		r.Names.Original + "ID",
		r.Names.Original + "Ids",
		r.Names.Original + "IDs",
		"Name",
		"Names",
		r.Names.Original + "Name",
		r.Names.Original + "Names",
	}

	// Handles field renames
	opType, _ := model.GetOpTypeAndResourceNameFromOpID(op.ExportedName, r.Config())
	renames := r.GetAllRenames(opType)
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

// FindARNIdentifiersInShape returns the identifier fields of a given shape which
// fit expect an ARN.
func FindARNIdentifiersInShape(
	r *model.CRD,
	shape *awssdkmodel.Shape,
) []string {
	var identifiers []string
	if r == nil || shape == nil {
		return identifiers
	}

	for _, memberName := range shape.MemberNames() {
		if r.IsPrimaryARNField(memberName) {
			identifiers = append(identifiers, memberName)
		}
	}

	return identifiers
}

// FindPluralizedIdentifiersInShape returns the name of a Spec OR Status field
// that has a matching pluralized field in the given shape and the name of
// the corresponding shape field name. This method handles identifier field
// renames and will return the same, when applicable.
// For example, DescribeVpcsInput has a `VpcIds` field which would be matched
// to the `Status.VPCID` CRD field - the return value would be
// "VPCID", "VpcIds".
func FindPluralizedIdentifiersInShape(
	r *model.CRD,
	shape *awssdkmodel.Shape,
	op *awssdkmodel.Operation,
) (crField string, shapeField string) {
	shapeIdentifiers := FindIdentifiersInShape(r, shape, op)
	crIdentifiers := r.GetIdentifiers()
	if len(shapeIdentifiers) == 0 || len(crIdentifiers) == 0 {
		return "", ""
	}

	pluralize := pluralize.NewClient()
	for _, si := range shapeIdentifiers {
		for _, ci := range crIdentifiers {
			// If the identifier field is renamed, we must take that into
			// consideration in order to find the corresponding matching
			// shapeIdentifier. Fetch field name from the config to apply
			// renames, if applicable
			siRenamed := r.Config().GetResourceFieldName(
				r.Names.Original,
				op.ExportedName,
				pluralize.Singular(si),
			)
			if strings.EqualFold(
				siRenamed,
				pluralize.Singular(ci),
			) {
				// The CRD identifiers being used for comparison reflect any
				// renamed field names in the API model shape.
				if crField == "" {
					crField = ci
					shapeField = si
				} else {
					// If there are multiple identifiers, then prioritize the
					// 'Id' identifier. Checking 'Id' to determine resource
					// creation should be safe as the field is usually
					// present in CR.Status.
					// Renames may produce "Id" or "ID" variants
					if !strings.Contains(strings.ToLower(crField), "id") {
						crField = ci
						shapeField = si
					}
				}
			}
		}
	}
	return crField, shapeField
}

// FindPrimaryIdentifierFieldNames returns the resource identifier field name
// for the primary identifier used in a given operation and its corresponding
// shape field name.
func FindPrimaryIdentifierFieldNames(
	cfg *ackgenconfig.Config,
	r *model.CRD,
	op *awssdkmodel.Operation,
) (crField string, shapeField string, err error) {
	shape := op.InputRef.Shape

	if shapeField == "" {
		// For ReadOne, search for a direct identifier
		if op == r.Ops.ReadOne {
			identifiers := FindIdentifiersInShape(r, shape, op)
			identifiers = append(identifiers, FindARNIdentifiersInShape(r, shape)...)

			switch len(identifiers) {
			case 0:
				break
			case 1:
				shapeField = identifiers[0]
			default:
				return "", "", fmt.Errorf(
					"resource %q: found multiple possible primary identifiers — set `is_primary_key` for the primary field",
					r.Names.Original,
				)
			}
		} else {
			// For ReadMany, search for pluralized identifiers
			crField, shapeField = FindPluralizedIdentifiersInShape(r, shape, op)
		}

		// Require override if still can't find any identifiers
		if shapeField == "" {
			return "", "", fmt.Errorf(
				"resource %q: could not find primary identifier — set `is_primary_key` for the primary field",
				r.Names.Original,
			)
		}
	}

	if r.IsPrimaryARNField(shapeField) {
		return "", PrimaryIdentifierARNOverride, nil
	}

	if crField == "" {
		if inSpec, inStat := r.HasMember(shapeField, op.ExportedName); !inSpec && !inStat {
			return "", "", fmt.Errorf(
				"resource %q: could not find corresponding spec or status field for primary identifier %q",
				r.Names.Original, shapeField,
			)
		}
		// Fetch field name from config to apply renames, if applicable
		crField = cfg.GetResourceFieldName(
			r.Names.Original,
			op.ExportedName,
			shapeField,
		)
	}
	return crField, shapeField, nil
}

// GetMemberIndex returns the index of memberName by iterating over
// shape's slice of member names for deterministic ordering
func GetMemberIndex(shape *awssdkmodel.Shape, memberName string) (int, error) {
	for index, shapeMemberName := range shape.MemberNames() {
		if strings.EqualFold(shapeMemberName, memberName) {
			return index, nil
		}
	}
	return -1, fmt.Errorf("Could not find %s in shape %s", memberName, shape.ShapeName)
}

// AdoptionIdentifier describes a field used to identify a resource during
// adoption. This is the shared representation used by both the Go code
// generator and the adoption metadata generator.
type AdoptionIdentifier struct {
	// Field is the CRD field for this identifier (nil for ARN-based primary keys)
	Field *model.Field
	// FieldName is the camelLower name used in the adoption annotation map key
	FieldName string
	// InSpec is true if the field is in Spec, false if in Status
	InSpec bool
	// IsPrimary is true if this is the primary identifier
	IsPrimary bool
	// PrimaryFromConfig is true when the field was explicitly designated as
	// primary via is_primary_key in generator.yaml (as opposed to auto-detected
	// from the operation's input shape). This distinction controls the generated
	// Go variable name ("primaryKey" vs "f{idx}") to avoid noisy diffs across
	// controller repos on regeneration.
	PrimaryFromConfig bool
	// Required is true if this field must be provided for adoption
	Required bool
}

// AdoptionFields is the result of analyzing a CRD's adoption requirements.
type AdoptionFields struct {
	// IsARNPrimary is true when the resource uses ARN as its sole identifier
	IsARNPrimary bool
	// Identifiers is the ordered list of fields needed for adoption.
	// The primary identifier is always first.
	Identifiers []AdoptionIdentifier
}

// GetAdoptionFields analyzes a CRD and returns the fields required to adopt
// an existing AWS resource. This is the single source of truth used by both
// PopulateResourceFromAnnotation (Go code generation) and the adoption
// metadata generator.
func GetAdoptionFields(
	cfg *ackgenconfig.Config,
	r *model.CRD,
) (*AdoptionFields, error) {
	op := r.Ops.ReadOne
	if op == nil {
		switch {
		case r.Ops.GetAttributes != nil:
			op = r.Ops.GetAttributes
		case r.Ops.ReadMany != nil:
			op = r.Ops.ReadMany
		default:
			return nil, nil
		}
	}
	inputShape := op.InputRef.Shape
	if inputShape == nil {
		return nil, nil
	}

	result := &AdoptionFields{}

	if r.IsARNPrimaryKey() {
		result.IsARNPrimary = true
		return result, nil
	}

	primaryField, err := r.GetPrimaryKeyField()
	if err != nil {
		return nil, err
	}

	var primaryCRField, primaryShapeField string
	isPrimarySet := primaryField != nil
	if !isPrimarySet {
		primaryCRField, primaryShapeField, err = FindPrimaryIdentifierFieldNames(cfg, r, op)
		if err != nil {
			return nil, err
		}
		if primaryShapeField == PrimaryIdentifierARNOverride {
			result.IsARNPrimary = true
			return result, nil
		}
	}

	paginatorFieldLookup := []string{
		"NextToken",
		"MaxResults",
	}

	for _, memberName := range inputShape.MemberNames() {
		if util.InStrings(memberName, paginatorFieldLookup) {
			continue
		}

		inputShapeRef := inputShape.MemberRefs[memberName]
		inputMemberShape := inputShapeRef.Shape

		if inputMemberShape.Type != "string" &&
			(inputMemberShape.Type != "list" ||
				inputMemberShape.MemberRef.Shape.Type != "string") {
			continue
		}

		if r.IsSecretField(memberName) {
			continue
		}

		if r.IsPrimaryARNField(memberName) {
			continue
		}

		fieldName := cfg.GetResourceFieldName(
			r.Names.Original,
			op.ExportedName,
			memberName,
		)

		if isPrimarySet && fieldName == primaryField.Names.Camel {
			continue
		}

		isPrimaryIdentifier := fieldName == primaryShapeField

		searchField := fieldName
		if isPrimaryIdentifier {
			searchField = primaryCRField
		}

		targetField := findFieldInSpec(cfg, r, searchField)
		if targetField == nil || (isPrimarySet && targetField == primaryField) {
			continue
		}

		switch targetField.ShapeRef.Shape.Type {
		case "list", "structure", "map":
			return nil, fmt.Errorf(
				"resource %q: primary identifier %q must be a scalar type since NameOrID is a string",
				r.Names.Original, targetField.Path,
			)
		}

		isRequired := inputShape.IsRequired(memberName) || isPrimaryIdentifier
		isPrimary := isPrimaryIdentifier

		result.Identifiers = append(result.Identifiers, AdoptionIdentifier{
			Field:     targetField,
			FieldName: targetField.Names.CamelLower,
			InSpec:    isFieldInSpec(r, targetField),
			IsPrimary: isPrimary,
			Required:  isRequired,
		})
	}

	// If the primary field was set via config, prepend it
	if isPrimarySet {
		primary := AdoptionIdentifier{
			Field:             primaryField,
			FieldName:         primaryField.Names.CamelLower,
			InSpec:            isFieldInSpec(r, primaryField),
			IsPrimary:         true,
			PrimaryFromConfig: true,
			Required:          true,
		}
		result.Identifiers = append([]AdoptionIdentifier{primary}, result.Identifiers...)
	}

	return result, nil
}

// findFieldInSpec searches for a field by name in the CRD's Spec and Status
// fields and returns it.
func findFieldInSpec(
	cfg *ackgenconfig.Config,
	r *model.CRD,
	searchField string,
) *model.Field {
	if f, ok := r.SpecFields[searchField]; ok {
		return f
	}
	if f, ok := r.StatusFields[searchField]; ok {
		return f
	}
	return nil
}

func isFieldInSpec(r *model.CRD, field *model.Field) bool {
	for _, f := range r.SpecFields {
		if f == field {
			return true
		}
	}
	return false
}
