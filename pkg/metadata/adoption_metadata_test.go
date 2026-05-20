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

package metadata_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/aws-controllers-k8s/code-generator/pkg/metadata"
	"github.com/aws-controllers-k8s/code-generator/pkg/testutil"
)

func TestGenerateAdoptionMetadata_SNS_ARNPrimary(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	g := testutil.NewModelForService(t, "sns")
	crds, err := g.GetCRDs()
	require.NoError(err)

	meta := metadata.GenerateAdoptionMetadata("sns", crds)
	require.NotNil(meta)
	assert.Equal("sns", meta.Service)

	var topic *metadata.AdoptionResource
	for i := range meta.Resources {
		if meta.Resources[i].Kind == "Topic" {
			topic = &meta.Resources[i]
			break
		}
	}
	require.NotNil(topic, "expected Topic in resources")
	assert.True(topic.Adoptable)
	require.NotNil(topic.PrimaryIdentifier)
	assert.Equal("arn", topic.PrimaryIdentifier.FieldName)
	assert.Equal("metadata", topic.PrimaryIdentifier.Location)
	assert.Equal("arn", topic.PrimaryIdentifier.Type)
	assert.True(topic.PrimaryIdentifier.Required)
	assert.Empty(topic.AdditionalKeys)
}

func TestGenerateAdoptionMetadata_SQS_PrimaryKeyFromConfig(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	g := testutil.NewModelForService(t, "sqs")
	crds, err := g.GetCRDs()
	require.NoError(err)

	meta := metadata.GenerateAdoptionMetadata("sqs", crds)
	require.NotNil(meta)

	var queue *metadata.AdoptionResource
	for i := range meta.Resources {
		if meta.Resources[i].Kind == "Queue" {
			queue = &meta.Resources[i]
			break
		}
	}
	require.NotNil(queue, "expected Queue in resources")
	assert.True(queue.Adoptable)
	require.NotNil(queue.PrimaryIdentifier)
	assert.Equal("queueURL", queue.PrimaryIdentifier.FieldName)
	assert.Equal("status", queue.PrimaryIdentifier.Location)
	assert.Equal("name", queue.PrimaryIdentifier.Type)
	assert.True(queue.PrimaryIdentifier.Required)
	assert.Empty(queue.AdditionalKeys)
}

func TestGenerateAdoptionMetadata_EC2_SecurityGroup_ReadMany(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	g := testutil.NewModelForService(t, "ec2")
	crds, err := g.GetCRDs()
	require.NoError(err)

	meta := metadata.GenerateAdoptionMetadata("ec2", crds)
	require.NotNil(meta)

	var sg *metadata.AdoptionResource
	for i := range meta.Resources {
		if meta.Resources[i].Kind == "SecurityGroup" {
			sg = &meta.Resources[i]
			break
		}
	}
	require.NotNil(sg, "expected SecurityGroup in resources")
	assert.True(sg.Adoptable)
	require.NotNil(sg.PrimaryIdentifier)
	assert.Equal("id", sg.PrimaryIdentifier.FieldName)
	assert.Equal("status", sg.PrimaryIdentifier.Location)
	assert.Equal("id", sg.PrimaryIdentifier.Type)
	assert.True(sg.PrimaryIdentifier.Required)
}

func TestGenerateAdoptionMetadata_EKS_WithDocOverrides(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	g := testutil.NewModelForServiceWithOptions(t, "eks", &testutil.TestingModelOptions{
		DocumentationConfigFile: "documentation-with-adoption.yaml",
	})
	crds, err := g.GetCRDs()
	require.NoError(err)

	meta := metadata.GenerateAdoptionMetadata("eks", crds)
	require.NotNil(meta)

	var fp *metadata.AdoptionResource
	for i := range meta.Resources {
		if meta.Resources[i].Kind == "FargateProfile" {
			fp = &meta.Resources[i]
			break
		}
	}
	require.NotNil(fp, "expected FargateProfile in resources")
	assert.True(fp.Adoptable)
	assert.Equal("ClusterName must match the cluster the controller is managing.", fp.Note)

	// The primary identifier should be Name (renamed from FargateProfileName)
	// and should be marked as not required due to the documentation override.
	require.NotNil(fp.PrimaryIdentifier)
	assert.Equal("name", fp.PrimaryIdentifier.FieldName)
	assert.False(fp.PrimaryIdentifier.Required)
	assert.Equal("Optional when using ARN-based adoption.", fp.PrimaryIdentifier.Note)

	// ClusterName should be in additional keys with a note
	require.NotEmpty(fp.AdditionalKeys)
	var clusterKey *metadata.AdoptionField
	for i := range fp.AdditionalKeys {
		if fp.AdditionalKeys[i].FieldName == "clusterName" {
			clusterKey = &fp.AdditionalKeys[i]
			break
		}
	}
	require.NotNil(clusterKey, "expected clusterName in additional keys")
	assert.Equal("The EKS cluster name. Must match the controller's configured cluster.", clusterKey.Note)
	assert.True(clusterKey.Required)
}

func TestGenerateAdoptionMetadata_APIGatewayV2_Api_ReadOne(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	g := testutil.NewModelForService(t, "apigatewayv2")
	crds, err := g.GetCRDs()
	require.NoError(err)

	meta := metadata.GenerateAdoptionMetadata("apigatewayv2", crds)
	require.NotNil(meta)

	var api *metadata.AdoptionResource
	for i := range meta.Resources {
		if meta.Resources[i].Kind == "API" {
			api = &meta.Resources[i]
			break
		}
	}
	require.NotNil(api, "expected API in resources")
	assert.True(api.Adoptable)
	require.NotNil(api.PrimaryIdentifier)
	assert.Equal("apiID", api.PrimaryIdentifier.FieldName)
	assert.Equal("status", api.PrimaryIdentifier.Location)
	assert.Equal("id", api.PrimaryIdentifier.Type)
	assert.True(api.PrimaryIdentifier.Required)
}

func TestWriteAdoptionMetadata_RoundTrip(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	meta := &metadata.AdoptionMetadata{
		Service: "test-service",
		Resources: []metadata.AdoptionResource{
			{
				Kind:      "Widget",
				Adoptable: true,
				Note:      "Test note",
				PrimaryIdentifier: &metadata.AdoptionField{
					FieldName: "widgetID",
					Location:  "status",
					Type:      "id",
					Required:  true,
				},
				AdditionalKeys: []metadata.AdoptionField{
					{
						FieldName: "name",
						Location:  "spec",
						Type:      "name",
						Required:  false,
						Note:      "Optional name field",
					},
				},
			},
			{
				Kind:      "NonAdoptable",
				Adoptable: false,
			},
		},
	}

	tmpDir := t.TempDir()
	outPath := filepath.Join(tmpDir, "adoption-metadata.json")

	err := metadata.WriteAdoptionMetadata(outPath, meta)
	require.NoError(err)

	data, err := os.ReadFile(outPath)
	require.NoError(err)

	var loaded metadata.AdoptionMetadata
	err = json.Unmarshal(data, &loaded)
	require.NoError(err)

	assert.Equal("test-service", loaded.Service)
	require.Len(loaded.Resources, 2)

	assert.Equal("Widget", loaded.Resources[0].Kind)
	assert.True(loaded.Resources[0].Adoptable)
	assert.Equal("Test note", loaded.Resources[0].Note)
	require.NotNil(loaded.Resources[0].PrimaryIdentifier)
	assert.Equal("widgetID", loaded.Resources[0].PrimaryIdentifier.FieldName)
	assert.Equal("status", loaded.Resources[0].PrimaryIdentifier.Location)
	assert.Equal("id", loaded.Resources[0].PrimaryIdentifier.Type)
	assert.True(loaded.Resources[0].PrimaryIdentifier.Required)
	require.Len(loaded.Resources[0].AdditionalKeys, 1)
	assert.Equal("name", loaded.Resources[0].AdditionalKeys[0].FieldName)
	assert.False(loaded.Resources[0].AdditionalKeys[0].Required)
	assert.Equal("Optional name field", loaded.Resources[0].AdditionalKeys[0].Note)

	assert.Equal("NonAdoptable", loaded.Resources[1].Kind)
	assert.False(loaded.Resources[1].Adoptable)
	assert.Nil(loaded.Resources[1].PrimaryIdentifier)
	assert.Empty(loaded.Resources[1].AdditionalKeys)
}
