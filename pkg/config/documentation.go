package config

import (
	"io/ioutil"

	"sigs.k8s.io/yaml"
)

// DocumentationConfig represents the configuration of the documentation file,
// used to override or append documentation to any of the resource fields
type DocumentationConfig struct {
	Resources map[string]*ResourceDocsConfig `json:"resources,omitempty"`
}

// ResourceDocsConfig represents the configuration for the documentation
// overrides of a single resource
type ResourceDocsConfig struct {
	Fields   map[string]*FieldDocsConfig `json:"fields,omitempty"`
	Adoption *AdoptionDocsConfig         `json:"adoption,omitempty"`
}

// AdoptionDocsConfig provides documentation overrides for a resource's
// adoption behavior. It allows marking auto-detected fields as optional or
// hidden, and adding descriptive notes.
type AdoptionDocsConfig struct {
	// Note is a human-readable description of adoption behavior for this
	// resource, shown in the API reference docs.
	Note string `json:"note,omitempty"`
	// Fields allows overriding properties of individual adoption fields.
	// Keys are the camelLower field names (e.g., "id", "name").
	Fields map[string]*AdoptionFieldDocsConfig `json:"fields,omitempty"`
}

// AdoptionFieldDocsConfig allows overriding an individual adoption field's
// documentation properties.
type AdoptionFieldDocsConfig struct {
	// Required overrides whether this field is shown as required. Set to
	// false for fields that are auto-resolved by hooks (e.g., ID resolved
	// from Name via a List call).
	Required *bool `json:"required,omitempty"`
	// Hidden removes the field from the adoption documentation entirely.
	Hidden bool `json:"hidden,omitempty"`
	// Note is a human-readable description shown alongside this field.
	Note string `json:"note,omitempty"`
}

// FieldDocsConfig represents the configuration for the documentation overrides
// of a single field
type FieldDocsConfig struct {
	// Append specifies a string that will be added to the end of the existing
	// GoDoc comment for the field
	Append *string `json:"append,omitempty"`
	// Prepend specifies a string that will be added before the existing
	// GoDoc comment for the field
	Prepend *string `json:"prepend,omitempty"`
	// Override will entirely replace the GoDoc comment for the field
	Override *string `json:"override,omitempty"`
}

// NewDocumentationConfig returns a new DocumentationConfig object given a
// supplied path to a config file
func NewDocumentationConfig(
	configPath string,
) (DocumentationConfig, error) {
	if configPath == "" {
		return DocumentationConfig{}, nil
	}
	content, err := ioutil.ReadFile(configPath)
	if err != nil {
		return DocumentationConfig{}, err
	}
	gc := DocumentationConfig{}
	if err = yaml.UnmarshalStrict(content, &gc); err != nil {
		return DocumentationConfig{}, err
	}
	return gc, nil
}
