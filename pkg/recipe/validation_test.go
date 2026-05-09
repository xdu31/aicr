// Copyright (c) 2026, NVIDIA CORPORATION & AFFILIATES.  All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package recipe

import (
	"encoding/json"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestToValidation(t *testing.T) {
	recipe := &RecipeResult{
		APIVersion: "aicr.nvidia.com/v1",
		Kind:       "RecipeResult",
		Metadata: struct {
			Version            string              `json:"version,omitempty" yaml:"version,omitempty"`
			AppliedOverlays    []string            `json:"appliedOverlays,omitempty" yaml:"appliedOverlays,omitempty"`
			ExcludedOverlays   []ExcludedOverlay   `json:"excludedOverlays,omitempty" yaml:"excludedOverlays,omitempty"`
			ConstraintWarnings []ConstraintWarning `json:"constraintWarnings,omitempty" yaml:"constraintWarnings,omitempty"`
		}{
			Version:         "1.0.0",
			AppliedOverlays: []string{"base", "eks"},
		},
		Criteria: &Criteria{
			Service:     CriteriaServiceEKS,
			Accelerator: CriteriaAcceleratorH100,
		},
		Constraints: []Constraint{
			{Name: "test-constraint", Value: "test-value"},
		},
		ComponentRefs: []ComponentRef{
			{Name: "gpu-operator"},
		},
		Validation: &ValidationConfig{
			Deployment: &ValidationPhase{
				Timeout: "10m",
			},
		},
	}

	validation := ToValidation(recipe)

	if validation == nil {
		t.Fatal("ToValidation returned nil")
	}

	// Verify optional resource fields are populated
	if validation.APIVersion != "aicr.nvidia.com/v1" {
		t.Errorf("APIVersion = %q, want %q", validation.APIVersion, "aicr.nvidia.com/v1")
	}
	if validation.Kind != KindValidation {
		t.Errorf("Kind = %q, want %q", validation.Kind, KindValidation)
	}
	if validation.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if validation.Metadata.Version != "1.0.0" {
		t.Errorf("Metadata.Version = %q, want %q", validation.Metadata.Version, "1.0.0")
	}

	// Verify validation fields are copied
	if validation.Criteria.Service != CriteriaServiceEKS {
		t.Errorf("Criteria.Service = %q, want %q", validation.Criteria.Service, CriteriaServiceEKS)
	}
	if len(validation.Constraints) != 1 {
		t.Errorf("len(Constraints) = %d, want 1", len(validation.Constraints))
	}
	if len(validation.ComponentRefs) != 1 {
		t.Errorf("len(ComponentRefs) = %d, want 1", len(validation.ComponentRefs))
	}
	if validation.Deployment == nil {
		t.Fatal("Deployment is nil")
	}
	if validation.Deployment.Timeout != "10m" {
		t.Errorf("Deployment.Timeout = %q, want %q", validation.Deployment.Timeout, "10m")
	}
}

func TestToValidationNil(t *testing.T) {
	validation := ToValidation(nil)
	if validation != nil {
		t.Errorf("ToValidation(nil) = %v, want nil", validation)
	}
}

func TestValidationJSONMarshal(t *testing.T) {
	validation := &Validation{
		APIVersion: "aicr.nvidia.com/v1",
		Kind:       KindValidation,
		Metadata: &ValidationMetadata{
			Name:    "test-validation",
			Version: "1.0.0",
		},
		ValidationConfig: ValidationConfig{
			Deployment: &ValidationPhase{
				Timeout: "10m",
				Checks:  []string{"check-1"},
			},
		},
		ComponentRefs: []ComponentRef{
			{Name: "gpu-operator"},
		},
		Criteria: Criteria{
			Service:     CriteriaServiceEKS,
			Accelerator: CriteriaAcceleratorH100,
		},
		Constraints: []Constraint{
			{Name: "test", Value: "value"},
		},
	}

	data, err := json.Marshal(validation)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}

	// Verify it contains expected fields
	jsonStr := string(data)
	expectedFields := []string{
		`"apiVersion":"aicr.nvidia.com/v1"`,
		`"kind":"Validation"`,
		`"name":"test-validation"`,
		`"version":"1.0.0"`,
	}
	for _, field := range expectedFields {
		if !contains(jsonStr, field) {
			t.Errorf("JSON missing field %s\nGot: %s", field, jsonStr)
		}
	}
}

func TestValidationJSONUnmarshal(t *testing.T) {
	jsonData := []byte(`{
		"apiVersion": "aicr.nvidia.com/v1",
		"kind": "Validation",
		"metadata": {
			"name": "test-validation",
			"version": "1.0.0"
		},
		"deployment": {
			"timeout": "10m",
			"checks": ["check-1"]
		},
		"componentRefs": [
			{"name": "gpu-operator"}
		],
		"criteria": {
			"service": "eks",
			"accelerator": "h100"
		},
		"constraints": [
			{"name": "test", "value": "value"}
		]
	}`)

	var validation Validation
	if err := json.Unmarshal(jsonData, &validation); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}

	if validation.APIVersion != "aicr.nvidia.com/v1" {
		t.Errorf("APIVersion = %q, want %q", validation.APIVersion, "aicr.nvidia.com/v1")
	}
	if validation.Kind != KindValidation {
		t.Errorf("Kind = %q, want %q", validation.Kind, KindValidation)
	}
	if validation.Metadata == nil {
		t.Fatal("Metadata is nil")
	}
	if validation.Metadata.Name != "test-validation" {
		t.Errorf("Metadata.Name = %q, want %q", validation.Metadata.Name, "test-validation")
	}
}

func TestValidationYAMLMarshal(t *testing.T) {
	validation := &Validation{
		APIVersion: "aicr.nvidia.com/v1",
		Kind:       KindValidation,
		Metadata: &ValidationMetadata{
			Name:    "test-validation",
			Version: "1.0.0",
		},
		ComponentRefs: []ComponentRef{
			{Name: "gpu-operator"},
		},
		Criteria: Criteria{
			Service:     CriteriaServiceEKS,
			Accelerator: CriteriaAcceleratorH100,
		},
	}

	data, err := yaml.Marshal(validation)
	if err != nil {
		t.Fatalf("yaml.Marshal() failed: %v", err)
	}

	yamlStr := string(data)
	expectedFields := []string{
		"apiVersion: aicr.nvidia.com/v1",
		"kind: Validation",
		"name: test-validation",
		"version: 1.0.0", // YAML may or may not quote string values
	}
	for _, field := range expectedFields {
		if !contains(yamlStr, field) {
			t.Errorf("YAML missing field %s\nGot: %s", field, yamlStr)
		}
	}
}

func TestValidationOmitEmpty(t *testing.T) {
	// Validation without optional fields - should serialize cleanly
	validation := &Validation{
		ComponentRefs: []ComponentRef{
			{Name: "gpu-operator"},
		},
		Criteria: Criteria{
			Service:     CriteriaServiceEKS,
			Accelerator: CriteriaAcceleratorH100,
		},
	}

	data, err := json.Marshal(validation)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}

	jsonStr := string(data)
	// Verify optional fields are omitted
	if contains(jsonStr, `"apiVersion"`) {
		t.Error("JSON should not contain apiVersion when empty")
	}
	if contains(jsonStr, `"kind"`) {
		t.Error("JSON should not contain kind when empty")
	}
	if contains(jsonStr, `"metadata"`) {
		t.Error("JSON should not contain metadata when nil")
	}

	// But required fields should be present
	if !contains(jsonStr, `"componentRefs"`) {
		t.Error("JSON should contain componentRefs")
	}
	if !contains(jsonStr, `"criteria"`) {
		t.Error("JSON should contain criteria")
	}
}

func TestValidationEmbedding(t *testing.T) {
	// Simulate embedding in a CR spec
	type ValidationJobSpec struct {
		Validation Validation `json:"validation"`
		Timeout    string     `json:"timeout"`
	}

	spec := ValidationJobSpec{
		Validation: Validation{
			// No APIVersion/Kind/Metadata - clean embedding
			ComponentRefs: []ComponentRef{
				{Name: "gpu-operator"},
			},
			Criteria: Criteria{
				Service:     CriteriaServiceEKS,
				Accelerator: CriteriaAcceleratorH100,
			},
		},
		Timeout: "30m",
	}

	data, err := json.Marshal(spec)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}

	jsonStr := string(data)
	// Verify clean embedding without resource metadata
	if contains(jsonStr, `"apiVersion"`) {
		t.Error("Embedded validation should not contain apiVersion")
	}
	if contains(jsonStr, `"kind"`) {
		t.Error("Embedded validation should not contain kind")
	}
}

// Helper function to check if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
