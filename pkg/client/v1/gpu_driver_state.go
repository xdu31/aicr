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
	"log/slog"

	"github.com/NVIDIA/aicr/pkg/measurement"
	"github.com/NVIDIA/aicr/pkg/recipe"
	"github.com/NVIDIA/aicr/pkg/snapshotter"
)

// gpuOperatorComponentName is the ComponentRef.Name used by the recipe
// registry for the NVIDIA GPU Operator. Kept in sync with
// recipes/registry.yaml — the auto-detect override only lands on the
// component that owns the driver.enabled Helm value.
const gpuOperatorComponentName = "gpu-operator"

// gpuHardwareSubtypeName is the subtype name emitted by
// pkg/collector/gpu when writing the driver-loaded reading (see the
// subtypeHardware constant there). Re-declared locally so this file
// does not pull the collector package.
const gpuHardwareSubtypeName = "hardware"

// gpuDriverState reports what the snapshot tells us about the NVIDIA
// kernel driver on the sampled GPU node.
//
// Cardinality note: the GPU collector runs on a single node the
// snapshotter Job schedules onto (via nvidia.com/gpu.present=true), so
// the state reflects that one sample. On homogeneous clusters — the
// common case — it is representative of every GPU pool. Mixed-pool
// clusters are out of scope for now and tracked separately (see #464).
type gpuDriverState int

const (
	// gpuDriverUnknown means we lack a signal — no snapshot, no GPU
	// measurement in the snapshot, or the driver-loaded reading is
	// absent. The injection path treats this as "don't touch anything"
	// so callers who never provide a snapshot see no behavior change.
	gpuDriverUnknown gpuDriverState = iota

	// gpuDriverNotObserved means the GPU measurement is present but the
	// hardware subtype reports no GPU on the sampled node
	// (gpu-present=false or gpu-count=0). No override — the recipe's
	// static provider defaults stand.
	gpuDriverNotObserved

	// gpuDriverPreinstalled means the sampled GPU node has the nvidia
	// kernel module loaded. Auto-inject driver.enabled=false to prevent
	// the GPU Operator from installing a second driver on top of the
	// one the platform (Azure --gpu-driver Install, a GPU-optimized
	// EKS AMI, GKE-COS, OKE bare-metal) has already provisioned.
	gpuDriverPreinstalled

	// gpuDriverAbsent means the sampled GPU node does not have the
	// nvidia kernel module loaded. No override — every provider
	// values.yaml either sets driver.enabled=true (base, inherited by
	// EKS/AKS) or is a documented preinstalled-driver overlay that
	// already carries driver.enabled=false statically. Overriding here
	// would only regress those.
	gpuDriverAbsent
)

// computeGPUDriverState reduces the snapshot to the single per-cluster
// signal used by the auto-detect override: is the NVIDIA driver already
// loaded on the sampled GPU node? The reducer is deliberately strict —
// a missing driver-loaded key returns Unknown, not Absent, so a stale
// snapshot produced by an older CLI cannot flip a hardened overlay.
func computeGPUDriverState(snap *snapshotter.Snapshot) gpuDriverState {
	if snap == nil || len(snap.Measurements) == 0 {
		return gpuDriverUnknown
	}

	var gpu *measurement.Measurement
	for _, m := range snap.Measurements {
		if m != nil && m.Type == measurement.TypeGPU {
			gpu = m
			break
		}
	}
	if gpu == nil {
		return gpuDriverUnknown
	}

	hw := gpu.GetSubtype(gpuHardwareSubtypeName)
	if hw == nil {
		return gpuDriverUnknown
	}

	// A hardware subtype that explicitly reports no GPU on the node
	// is a distinct signal from "we couldn't tell" — the sampled node
	// simply is not a GPU node. Bucket separately so it cannot be
	// confused with a driver-loaded=false GPU node.
	if present := hw.Get(measurement.KeyGPUPresent); present != nil {
		if b, ok := present.Any().(bool); ok && !b {
			return gpuDriverNotObserved
		}
	}
	if count := hw.Get(measurement.KeyGPUCount); count != nil {
		switch v := count.Any().(type) {
		case int:
			if v == 0 {
				return gpuDriverNotObserved
			}
		case int64:
			if v == 0 {
				return gpuDriverNotObserved
			}
		}
	}

	loaded := hw.Get(measurement.KeyGPUDriverLoaded)
	if loaded == nil {
		return gpuDriverUnknown
	}
	b, ok := loaded.Any().(bool)
	if !ok {
		return gpuDriverUnknown
	}
	if b {
		return gpuDriverPreinstalled
	}
	return gpuDriverAbsent
}

// applyGPUDriverAutoOverride injects driver.enabled=false into the
// gpu-operator ComponentRef's Overrides map when the snapshot reports
// a pre-installed driver on the sampled GPU node.
//
// Policy is only-false: the function never forces driver.enabled=true.
// That keeps the change safe across every provider — GKE-COS and OKE
// values files already carry driver.enabled=false, so the injection is
// byte-for-byte idempotent for them; AKS with --gpu-driver Install and
// EKS on preinstalled-driver AMIs get the fix; every other state
// (Absent, NotObserved, Unknown) is a no-op so callers who never pass a
// snapshot see zero behavior change.
//
// Merge precedence is base values.yaml → ValuesFile → Overrides (highest,
// see pkg/recipe/adapter.go), so writing driver.enabled into Overrides
// wins over every provider values file at bundle time.
func applyGPUDriverAutoOverride(r *recipe.RecipeResult, state gpuDriverState) {
	if r == nil || state != gpuDriverPreinstalled {
		return
	}
	for i := range r.ComponentRefs {
		if r.ComponentRefs[i].Name != gpuOperatorComponentName {
			continue
		}
		ref := &r.ComponentRefs[i]
		if ref.Overrides == nil {
			ref.Overrides = map[string]any{}
		}
		driverAny, _ := ref.Overrides["driver"].(map[string]any)
		if driverAny == nil {
			driverAny = map[string]any{}
			ref.Overrides["driver"] = driverAny
		}
		driverAny["enabled"] = false
		slog.Info("auto-disabled gpu-operator driver install: pre-installed driver detected in snapshot",
			"component", gpuOperatorComponentName,
			"reason", "driver-loaded=true")
		return
	}
}
