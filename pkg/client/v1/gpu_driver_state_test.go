// Copyright (c) 2026, NVIDIA CORPORATION.  All rights reserved.
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

package aicr

import (
	"testing"

	"github.com/NVIDIA/aicr/pkg/measurement"
	"github.com/NVIDIA/aicr/pkg/recipe"
	"github.com/NVIDIA/aicr/pkg/snapshotter"
)

func TestComputeGPUDriverState(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		snap *snapshotter.Snapshot
		want gpuDriverState
	}{
		{
			name: "nil snapshot",
			snap: nil,
			want: gpuDriverUnknown,
		},
		{
			name: "empty measurements",
			snap: &snapshotter.Snapshot{},
			want: gpuDriverUnknown,
		},
		{
			name: "k8s-only snapshot has no GPU measurement",
			snap: &snapshotter.Snapshot{
				Measurements: []*measurement.Measurement{
					measurement.NewMeasurement(measurement.TypeK8s).
						WithSubtypeBuilder(
							measurement.NewSubtypeBuilder("server").
								SetString(measurement.KeyVersion, "v1.33.0"),
						).
						Build(),
				},
			},
			want: gpuDriverUnknown,
		},
		{
			name: "GPU measurement with no subtypes",
			snap: &snapshotter.Snapshot{
				Measurements: []*measurement.Measurement{
					{Type: measurement.TypeGPU},
				},
			},
			want: gpuDriverUnknown,
		},
		{
			name: "hardware subtype without driver-loaded key",
			snap: gpuHardwareSnapshotWith(func(b *measurement.SubtypeBuilder) {
				b.SetBool(measurement.KeyGPUPresent, true).
					SetInt(measurement.KeyGPUCount, 8)
			}),
			want: gpuDriverUnknown,
		},
		{
			name: "gpu-present=false",
			snap: gpuHardwareSnapshotWith(func(b *measurement.SubtypeBuilder) {
				b.SetBool(measurement.KeyGPUPresent, false).
					SetInt(measurement.KeyGPUCount, 0).
					SetBool(measurement.KeyGPUDriverLoaded, false)
			}),
			want: gpuDriverNotObserved,
		},
		{
			name: "gpu-count=0",
			snap: gpuHardwareSnapshotWith(func(b *measurement.SubtypeBuilder) {
				b.SetBool(measurement.KeyGPUPresent, true).
					SetInt(measurement.KeyGPUCount, 0).
					SetBool(measurement.KeyGPUDriverLoaded, true)
			}),
			want: gpuDriverNotObserved,
		},
		{
			name: "driver-loaded=true",
			snap: gpuHardwareSnapshotWith(func(b *measurement.SubtypeBuilder) {
				b.SetBool(measurement.KeyGPUPresent, true).
					SetInt(measurement.KeyGPUCount, 8).
					SetBool(measurement.KeyGPUDriverLoaded, true)
			}),
			want: gpuDriverPreinstalled,
		},
		{
			name: "driver-loaded=false",
			snap: gpuHardwareSnapshotWith(func(b *measurement.SubtypeBuilder) {
				b.SetBool(measurement.KeyGPUPresent, true).
					SetInt(measurement.KeyGPUCount, 8).
					SetBool(measurement.KeyGPUDriverLoaded, false)
			}),
			want: gpuDriverAbsent,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := computeGPUDriverState(tt.snap); got != tt.want {
				t.Errorf("computeGPUDriverState() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestApplyGPUDriverAutoOverride(t *testing.T) {
	t.Parallel()

	makeResult := func(refs ...recipe.ComponentRef) *recipe.RecipeResult {
		return &recipe.RecipeResult{ComponentRefs: refs}
	}
	gpuOp := func(overrides map[string]any) recipe.ComponentRef {
		return recipe.ComponentRef{Name: gpuOperatorComponentName, Overrides: overrides}
	}

	tests := []struct {
		name         string
		result       *recipe.RecipeResult
		state        gpuDriverState
		wantInjected bool // driver.enabled key present under Overrides
		wantValue    bool // value of driver.enabled (only checked when injected)
	}{
		{
			name:         "nil result is a no-op",
			result:       nil,
			state:        gpuDriverPreinstalled,
			wantInjected: false,
		},
		{
			name:         "state=Unknown never injects",
			result:       makeResult(gpuOp(nil)),
			state:        gpuDriverUnknown,
			wantInjected: false,
		},
		{
			name:         "state=NotObserved never injects",
			result:       makeResult(gpuOp(nil)),
			state:        gpuDriverNotObserved,
			wantInjected: false,
		},
		{
			name:         "state=Absent never injects (only-false policy)",
			result:       makeResult(gpuOp(nil)),
			state:        gpuDriverAbsent,
			wantInjected: false,
		},
		{
			name:         "Preinstalled + empty Overrides injects false",
			result:       makeResult(gpuOp(nil)),
			state:        gpuDriverPreinstalled,
			wantInjected: true,
			wantValue:    false,
		},
		{
			name: "Preinstalled + existing sibling override preserves siblings",
			result: makeResult(gpuOp(map[string]any{
				"toolkit": map[string]any{"enabled": true},
			})),
			state:        gpuDriverPreinstalled,
			wantInjected: true,
			wantValue:    false,
		},
		{
			name: "Preinstalled overrides an existing driver.enabled=true",
			result: makeResult(gpuOp(map[string]any{
				"driver": map[string]any{"enabled": true},
			})),
			state:        gpuDriverPreinstalled,
			wantInjected: true,
			wantValue:    false,
		},
		{
			name:         "no gpu-operator ref is a no-op",
			result:       makeResult(recipe.ComponentRef{Name: "nvsentinel"}),
			state:        gpuDriverPreinstalled,
			wantInjected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			applyGPUDriverAutoOverride(tt.result, tt.state)

			if tt.result == nil {
				return
			}
			var got any
			for _, ref := range tt.result.ComponentRefs {
				if ref.Name != gpuOperatorComponentName {
					continue
				}
				driver, _ := ref.Overrides["driver"].(map[string]any)
				if driver != nil {
					got = driver["enabled"]
				}
			}
			if !tt.wantInjected {
				if got != nil {
					t.Errorf("driver.enabled = %v, want no injection", got)
				}
				return
			}
			b, ok := got.(bool)
			if !ok {
				t.Fatalf("driver.enabled = %v (%T), want bool %v", got, got, tt.wantValue)
			}
			if b != tt.wantValue {
				t.Errorf("driver.enabled = %v, want %v", b, tt.wantValue)
			}
		})
	}
}

func TestApplyGPUDriverAutoOverride_PreservesSiblingsUnderDriver(t *testing.T) {
	t.Parallel()

	// A pre-existing sibling under driver (e.g. driver.version) must not be
	// clobbered by the enabled=false injection — only the enabled key changes.
	result := &recipe.RecipeResult{
		ComponentRefs: []recipe.ComponentRef{
			{
				Name: gpuOperatorComponentName,
				Overrides: map[string]any{
					"driver": map[string]any{
						"version": "580.126.20",
					},
				},
			},
		},
	}
	applyGPUDriverAutoOverride(result, gpuDriverPreinstalled)

	driver, ok := result.ComponentRefs[0].Overrides["driver"].(map[string]any)
	if !ok {
		t.Fatalf("Overrides[\"driver\"] = %v, want map[string]any", result.ComponentRefs[0].Overrides["driver"])
	}
	if driver["version"] != "580.126.20" {
		t.Errorf("driver.version = %v, want preserved 580.126.20", driver["version"])
	}
	if driver["enabled"] != false {
		t.Errorf("driver.enabled = %v, want false", driver["enabled"])
	}
}

// gpuHardwareSnapshotWith builds a minimal snapshot with a single GPU
// measurement carrying a "hardware" subtype the caller populates through
// the passed builder callback. Colocated with the reducer tests because
// the aicr_test.go helper (package aicr_test) cannot reach unexported
// symbols in this file; the wire-up tests over there use their own
// gpuHardwareSnapshot() constructor.
func gpuHardwareSnapshotWith(fill func(*measurement.SubtypeBuilder)) *snapshotter.Snapshot {
	sb := measurement.NewSubtypeBuilder(gpuHardwareSubtypeName)
	fill(sb)
	return &snapshotter.Snapshot{
		Measurements: []*measurement.Measurement{
			measurement.NewMeasurement(measurement.TypeGPU).
				WithSubtypeBuilder(sb).
				Build(),
		},
	}
}
