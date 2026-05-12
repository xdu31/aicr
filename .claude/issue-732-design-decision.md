# Issue #732: Extract Validation as Standalone Portable Type - Design Decision

**Date:** 2026-05-08
**Issue:** https://github.com/NVIDIA/aicr/issues/732
**Parent Issue:** #731 - Support external validation controller integration
**ADR Context:** https://gitlab-master.nvidia.com/dgxcloud/platform/architecture/-/merge_requests/64

## Executive Summary

**Decision:** Create `pkg/types` package for shared portable types, moving `ValidationConfig`, `Snapshot`, `ComponentRef`, `Criteria`, and `Constraint` out of `pkg/recipe` and `pkg/snapshotter`. Add new `Validation` type that combines `ValidationConfig` + `ValidationContext` as the complete validation specification.

**Why:** Enables external consumers (Validation Controller) to import clean, portable types without pulling in recipe-specific or snapshotter-specific logic. Creates clear evolution path from `RecipeResult` to `Validation` as the validation input contract, and provides single import point for all AICR types.

## Problem Statement

### Issue #732 Requirements
Extract `Validation` (currently `ValidationConfig` in `pkg/recipe/metadata.go`) as standalone, portable Go type with:
1. `NodeSelector map[string]string` for node targeting
2. `ExpectedNewReplicas int` for batch processing
3. Status tracking fields (phase state, check results)
4. Backward compatibility with existing recipe structure
5. JSON/YAML tags for serialization
6. No external dependencies (portable)

### Parent Issue #731 Context
Enable external consumers to use AICR as validation planning engine:
- Controller calls AICR planning functions (not CLI)
- AICR returns Job specs without deploying them
- Controller handles deployment, sidecars, orchestration
- Need portable types that can be embedded in CRDs

### ADR MR !64 Integration Pattern
Validation Controller embeds AICR types in CRD specs:
```go
type ValidationSpec struct {
    recipe.ValidationConfig `json:",inline"`  // Embedded AICR type
    NodeSelector         map[string]string   // Controller field
    ExpectedNewReplicas int                  // Controller field
}
```

Controller builds RecipeResult from Criteria, then passes to AICR planning functions.

## Key Discovery: Validators Need More Than ValidationConfig

### Current Validator Dependencies
```go
// validators/context.go
type Context struct {
    Recipe *recipe.RecipeResult  // Full recipe
}

// Validators access:
ctx.Recipe.Validation.Performance.Constraints     // ValidationConfig
ctx.Recipe.ComponentRefs                          // What's deployed
ctx.Recipe.Criteria.Accelerator                   // Cluster type
```

**Finding:** Validators need:
- ✅ `ValidationConfig` - validation phases/checks/constraints
- ✅ `ComponentRefs` - what components to validate
- ✅ `Criteria` - cluster characteristics for thresholds

Just extracting ValidationConfig is **not sufficient**.

## Evolution of Design Thinking

### Iteration 1: Extract ValidationConfig Only
**Proposal:** Move ValidationConfig to new `pkg/validation` package.

**Problem:** Validators still need ComponentRefs and Criteria. Either:
- Controller must build full RecipeResult (weird - controller doesn't have recipe YAML)
- Validators must be refactored to not need ComponentRefs/Criteria (large change)

### Iteration 2: ValidationContext as Runtime Wrapper
**Proposal:**
```go
type ValidationConfig struct { ... }  // Existing
type ValidationContext struct {       // New wrapper
    Validation    *ValidationConfig
    ComponentRefs []ComponentRef
    Criteria      *Criteria
}
```

**Problem:** Where do ComponentRefs and Criteria come from?
- Current AICR: From recipe YAML files (base + overlays)
- Controller: Doesn't have recipe YAML files

**Solution:** Controller uses AICR recipe builder:
```go
recipe := recipeBuilder.BuildFromCriteria(ctx, &cr.Spec.Criteria)
validationCtx := &ValidationContext{
    Validation:    &cr.Spec.ValidationConfig,
    ComponentRefs: recipe.ComponentRefs,  // From recipe builder
    Criteria:      &cr.Spec.Criteria,     // From CR
}
```

### Iteration 3: Separate Config from Context
**Proposal:**
- `ValidationConfig` = what validation to run (phases/checks/constraints)
- `ValidationContext` = what to validate (componentRefs/criteria)
- Both embedded inline in CRD

```go
type ValidationSpec struct {
    ValidationConfig  `json:",inline"`   // deployment, performance, conformance
    ValidationContext `json:",inline"`   // componentRefs, criteria
    NodeSelector      map[string]string  // Controller orchestration
}
```

**Benefit:** Clear separation of concerns.

### Iteration 4: Parent Validation Type (CHOSEN)
**Proposal:**
```go
type Validation struct {
    ValidationConfig  `json:",inline"`   // What to run
    ValidationContext `json:",inline"`   // What to validate
}
```

**Benefits:**
- ✅ Single type for complete validation specification
- ✅ Clean evolution: `RecipeResult` → `Validation`
- ✅ Controller embeds one type, not two
- ✅ Future: standalone validation YAML files (not recipes)
- ✅ RecipeResult has extras validators don't need (DeploymentOrder, Metadata, etc.)

## Package Organization Discussion

### Option 1: Keep in pkg/recipe
**Reasoning:**
- ValidationConfig is part of RecipeResult
- Recipe YAML files define validation sections
- Recipes are source of truth for validation config
- Natural dependency: validator → recipe (not recipe → validator)

**Against:**
- External consumers import `pkg/recipe` (sounds heavy)
- Validation types mixed with recipe building logic

### Option 2: Move to pkg/validator
**Reasoning:**
- Semantic alignment (validation package owns validation types)
- Validators use these types

**Against:**
- ❌ Circular dependency: pkg/recipe needs ValidationConfig for RecipeResult.Validation
- ❌ Recipe YAML files define validation (recipes own the schema, not validators)
- ❌ Backwards dependency direction

### Option 3: Create pkg/types (CHOSEN)
**Reasoning:**
- ✅ Clean separation of portable types vs recipe/validator logic
- ✅ Both pkg/recipe and pkg/validator import pkg/types
- ✅ External consumers import just pkg/types
- ✅ Clear ownership: types are neutral, recipe and validator consume them

**Structure:**
```
pkg/types/
  ├── validation.go    (ValidationConfig, ValidationPhase, NodeSelection,
  │                     ValidationContext, Validation)
  ├── snapshot.go      (Snapshot, Measurement, Header)
  ├── component.go     (ComponentRef, ComponentType, ExpectedResource)
  ├── constraint.go    (Constraint)
  └── criteria.go      (Criteria)

pkg/recipe/
  ├── metadata.go      (RecipeMetadata, RecipeResult - import types.*)
  ├── builder.go       (recipe building logic)
  └── convert.go       (ValidationFromRecipe() helper)

pkg/snapshotter/
  ├── snapshot.go      (collection logic - import types.Snapshot)
  └── agent.go

pkg/validator/
  └── validator.go     (validation execution - import types.*)
```

## Final Design: Option 3 + Snapshot

### Type Definitions

```go
// pkg/types/validation.go

type ValidationConfig struct {
    Deployment  *ValidationPhase `json:"deployment,omitempty"`
    Performance *ValidationPhase `json:"performance,omitempty"`
    Conformance *ValidationPhase `json:"conformance,omitempty"`
}

type ValidationPhase struct {
    Timeout        string            `json:"timeout,omitempty"`
    Constraints    []Constraint      `json:"constraints,omitempty"`
    Checks         []string          `json:"checks,omitempty"`
    NodeSelection  *NodeSelection    `json:"nodeSelection,omitempty"`
    Infrastructure string            `json:"infrastructure,omitempty"`
}

type NodeSelection struct {
    Selector     map[string]string `json:"selector,omitempty"`
    MaxNodes     int               `json:"maxNodes,omitempty"`
    ExcludeNodes []string          `json:"excludeNodes,omitempty"`
}

type ValidationContext struct {
    ComponentRefs []ComponentRef `json:"componentRefs"`
    Criteria      Criteria       `json:"criteria"`
}

// Validation is the complete validation specification (concrete struct).
// Used by both CLI (via Recipe.ToValidation()) and Controller (direct creation).
type Validation struct {
    ValidationConfig  `json:",inline"`
    ValidationContext `json:",inline"`
}
```

```go
// pkg/types/snapshot.go

// Snapshot represents collected cluster state
type Snapshot struct {
    Header       Header         `json:",inline"`
    Measurements []Measurement  `json:"measurements"`
}

type Measurement struct {
    Name     string                 `json:"name"`
    Value    string                 `json:"value"`
    Unit     string                 `json:"unit,omitempty"`
    Metadata map[string]string      `json:"metadata,omitempty"`
}

type Header struct {
    Kind       string            `json:"kind"`
    APIVersion string            `json:"apiVersion"`
    Metadata   map[string]string `json:"metadata,omitempty"`
}
```

### Controller CRD

```yaml
apiVersion: aicr.nvidia.com/v1alpha1
kind: Validation
spec:
  # ValidationConfig (inline via Validation)
  deployment:
    checks: [operator-health]
  performance:
    checks: [nccl-all-reduce-bw]

  # ValidationContext (inline via Validation)
  componentRefs:
    - name: gpu-operator
      version: v25.10.1
  criteria:
    service: eks
    accelerator: h100

  # Controller orchestration (not part of Validation)
  nodeSelector: {...}
  expectedNewReplicas: 8
```

```go
// Controller Go types
type ValidationControllerSpec struct {
    // Embeds Validation inline (concrete struct)
    types.Validation `json:",inline"`

    // Controller orchestration fields
    NodeSelector        map[string]string `json:"nodeSelector,omitempty"`
    ExpectedNewReplicas int               `json:"expectedNewReplicas,omitempty"`
}
```

### API Evolution

**Current (v1):**
```go
ValidatePhases(ctx, phases, recipeResult, snapshot) ([]*PhaseResult, error)
// recipeResult is *recipe.RecipeResult
```

**After refactoring (breaking change - pre-v1.0):**
```go
ValidatePhases(ctx, phases, validation, snapshot) ([]*PhaseResult, error)
// validation is *types.Validation (concrete struct)

// Call sites must update:
// OLD: ValidatePhases(ctx, phases, recipe, snapshot)
// NEW: ValidatePhases(ctx, phases, recipe.ToValidation(), snapshot)
```

**Future (v2 - new planning API):**
```go
BuildJobPlan(ctx, validation, snapshot, catalog) ([]Job, error)
// validation is *types.Validation
```

### Data Flow

**AICR CLI (explicit conversion):**
```
recipe.yaml → RecipeResult
    ↓
recipe.ToValidation() → Validation
    ↓
ValidatePhases(ctx, phases, validation, snapshot)
    ↓
Direct field access: validation.Deployment, validation.ComponentRefs, etc.
```

**Controller (direct creation):**
```
Validation CR
    ↓
Extract Validation from CR.Spec
    ↓
BuildJobPlan(ctx, validation, snapshot, catalog)
    ↓
Direct field access: validation.Deployment, validation.ComponentRefs, etc.
```

**How Controller Gets ComponentRefs:**
```
1. User specifies Criteria in CR
2. Controller calls AICR recipe builder: BuildFromCriteria(criteria)
3. Recipe builder returns RecipeResult with ComponentRefs
4. Controller extracts ComponentRefs from RecipeResult
5. Controller creates Validation with ComponentRefs + ValidationConfig
6. Controller calls AICR with Validation
```

## Recipe/Validation Compatibility

**Question:** Can Recipe be compatible with Validation to avoid breaking changes?

**Answer:** Using explicit conversion - Recipe has a `.ToValidation()` method.

### Design Decision: Explicit Conversion with Helper Method

**Validation is a concrete struct** (not an interface):

```go
// pkg/types/validation.go

type ValidationConfig struct {
    Deployment  *ValidationPhase `json:"deployment,omitempty"`
    Performance *ValidationPhase `json:"performance,omitempty"`
    Conformance *ValidationPhase `json:"conformance,omitempty"`
}

type ValidationContext struct {
    ComponentRefs []ComponentRef `json:"componentRefs"`
    Criteria      Criteria       `json:"criteria"`
}

// Validation is the complete validation specification (concrete struct)
type Validation struct {
    ValidationConfig  `json:",inline"`
    ValidationContext `json:",inline"`
}
```

**Recipe provides conversion method:**

```go
// pkg/recipe/recipe.go

// ToValidation converts RecipeResult to Validation for use with validators.
func (r *RecipeResult) ToValidation() *types.Validation {
    if r.Validation == nil {
        return &types.Validation{
            ValidationContext: types.ValidationContext{
                ComponentRefs: r.ComponentRefs,
                Criteria:      r.Criteria,
            },
        }
    }

    return &types.Validation{
        ValidationConfig: *r.Validation,
        ValidationContext: types.ValidationContext{
            ComponentRefs: r.ComponentRefs,
            Criteria:      r.Criteria,
        },
    }
}
```

### Migration Pattern

**Old code (before refactoring):**
```go
// validators/context.go
type Context struct {
    Recipe *recipe.RecipeResult
}

// CLI
validator.ValidatePhases(ctx, phases, recipe, snapshot)
```

**New code (after refactoring):**
```go
// validators/context.go
type Context struct {
    Validation *types.Validation  // Changed from Recipe
}

// CLI (explicit conversion)
validator.ValidatePhases(ctx, phases, recipe.ToValidation(), snapshot)
```

**Controller (creates Validation directly):**
```go
// Controller builds Validation from CR spec
validation := &types.Validation{
    ValidationConfig: cr.Spec.ValidationConfig,
    ValidationContext: types.ValidationContext{
        ComponentRefs: componentRefs,  // From recipe builder
        Criteria:      cr.Spec.Criteria,
    },
}

// Call AICR
validator.ValidatePhases(ctx, phases, validation, snapshot)
```

### Benefits

**1. Simple and Clear**
- No interface complexity
- Explicit conversion point visible in code
- Easy to understand data flow

**2. Type Safety**
- Validators work with concrete `*types.Validation` type
- No method indirection
- Compile-time type checking

**3. Clean Controller Integration**
```go
// Controller CRD embeds Validation inline
type ValidationControllerSpec struct {
    types.Validation `json:",inline"`  // Concrete struct
    NodeSelector     map[string]string `json:"nodeSelector,omitempty"`
}
```

### Trade-offs

**Pros:**
- ✅ Simple design - concrete struct, no interfaces
- ✅ Clear conversion - `.ToValidation()` is explicit
- ✅ No method indirection - direct field access
- ✅ Easy to debug - concrete types throughout

**Cons:**
- ❌ Breaking change - call sites must update from `recipe` to `recipe.ToValidation()`
- Memory overhead - creates new struct (but minimal)

**Decision:** Acceptable trade-off since AICR is pre-v1.0 (breaking changes allowed).

## Implementation Plan

### Phase 1: Create pkg/types (7 commits)

1. **Create package structure**
   ```bash
   mkdir -p pkg/types
   touch pkg/types/{validation,snapshot,component,constraint,criteria}.go
   ```

2. **Move Criteria** (pkg/recipe → pkg/types)
   - Copy type definition
   - Update pkg/recipe imports
   - Update all references
   - Remove old definition

3. **Move Constraint** (pkg/recipe → pkg/types)

4. **Move ComponentRef + ComponentType** (pkg/recipe → pkg/types)

5. **Move ValidationConfig + ValidationPhase + NodeSelection** (pkg/recipe → pkg/types)

6. **Move Snapshot + Measurement + Header** (pkg/snapshotter → pkg/types)
   - Copy `Snapshot`, `Measurement` types
   - Copy `Header` type (or create in pkg/types)
   - Update pkg/snapshotter to import types.Snapshot
   - Update pkg/validator to import types.Snapshot
   - Update all snapshot deserialization calls
   - Remove old definitions

7. **Add ValidationContext + Validation** (new in pkg/types)
   - Add `ValidationContext` struct
   - Add `Validation` struct (embeds ValidationConfig + ValidationContext)
   - Add `RecipeResult.ToValidation()` method in pkg/recipe/recipe.go

### Phase 2: Update Function Signatures (3 commits)

8. **Update pkg/validator**
   - `ValidatePhases()`: `*RecipeResult` → `*types.Validation`, `*snapshotter.Snapshot` → `*types.Snapshot`
   - `ensureDataConfigMaps()`: `*RecipeResult` → `*types.Validation`
   - `validators.Context`: `Recipe *recipe.RecipeResult` → `Validation *types.Validation`

9. **Update validators**
   - Change all `ctx.Recipe.*` to `ctx.Validation.*`
   - Update 10+ validator implementations
   - Update snapshot references to use `types.Snapshot`

10. **Update CLI**
    - `pkg/cli/validate.go`: Use `ValidationFromRecipe()`
    - `pkg/cli/snapshot.go`: Use `types.Snapshot` for serialization

### Phase 3: Tests + Documentation (2 commits)

11. **Update tests**
    - Fix compilation errors
    - Update test data structures
    - Update snapshot test fixtures

12. **Update documentation**
    - Update README, CONTRIBUTING.md
    - Document pkg/types package (Validation + Snapshot)
    - Update issue #732
    - Add migration guide for external consumers

## Scope Impact

**Original #732 scope:** Small (add ValidationContext wrapper)

**Updated scope (Option 3 + Snapshot):** Large (full package refactoring)

**Estimated effort:**
- ~15-20 files changed
- ~700-1000 lines moved
- ~300-400 import updates
- 12 commits across 3 phases

**Types moved to pkg/types:**
- Validation types: ValidationConfig, ValidationPhase, NodeSelection, ValidationContext, Validation
- Snapshot types: Snapshot, Measurement, Header
- Recipe types: Criteria, ComponentRef, ComponentType, Constraint, ExpectedResource

**Risk:** Breaking changes for external consumers (if any exist pre-v1.0)

**Mitigation:**
- AICR is pre-v1.0 (currently v0.x)
- Breaking changes acceptable before v1.0
- Clean evolution path for future stability

## Snapshot Dependency Analysis

### Discovery: Snapshot is Recipe-Independent ✅

**Question:** Does snapshot collection depend on recipe?

**Answer:** **No.** Snapshot is completely independent of recipe.

**Current CLI:**
```bash
# No --recipe flag exists
aicr snapshot --output snapshot.yaml

# Available flags (all recipe-independent):
--output              # Where to write snapshot
--node-selector       # Which nodes to collect from
--toleration          # Tolerations for agent pod
--template            # Output template
--namespace           # Agent deployment namespace
```

**Snapshot collection function:**
```go
// pkg/snapshotter/snapshot.go
func (n *NodeSnapshotter) Measure(ctx context.Context) error {
    // NO recipe parameter - just collects cluster state
}
```

**What snapshot collects:**
- ✅ Kubernetes version, node info
- ✅ GPU hardware (nvidia-smi)
- ✅ Network topology
- ✅ Installed components (discovers what's deployed)
- ✅ OS configuration

**What snapshot does NOT use:**
- ❌ Recipe (no dependency)
- ❌ ComponentRefs (discovers components dynamically)
- ❌ Criteria (records actual cluster, not expected)
- ❌ Validation config (just state collection)

**Why this matters:**
```
Snapshot = "What IS in cluster"     (actual state - observation)
Recipe   = "What SHOULD be deployed" (desired state - specification)
Validation = Compare(Snapshot, Recipe) (actual vs expected)
```

### Snapshot Type Ownership Decision

**Current structure:**
```go
// pkg/snapshotter/types.go
type Snapshot struct {
    header.Header `json:",inline"`
    Measurements []*measurement.Measurement
}
```

**Question:** Should Validation Controller import `pkg/snapshotter`, or should Snapshot move to `pkg/types`?

**Three options evaluated:**

| Option | Approach | Pros | Cons |
|--------|----------|------|------|
| 1 | Controller imports pkg/snapshotter | No duplication | Controller imports AICR internals |
| 2 | Move Snapshot to pkg/types | Clean separation, consistent | Larger refactoring |
| 3 | Store in ConfigMap, reference in CR | Lightweight CR | Two-step fetch |

**Decision: Option 2 - Move Snapshot to pkg/types**

**Rationale:**
1. **Consistent with pkg/types strategy**: Validation → pkg/types, Snapshot → pkg/types
2. **Clean controller imports**: Controller only imports `pkg/types`, not mixed imports
3. **Snapshot is portable**: Plain Go struct, JSON/YAML tags, no K8s dependencies
4. **Direct CRD embedding**: `types.Snapshot` can be embedded in Snapshot CR status
5. **Future-proof**: External consumers import just `pkg/types` for all AICR types

**Updated package structure:**
```
pkg/types/
  ├── validation.go    (Validation types)
  ├── snapshot.go      (Snapshot, Measurement, Header)  ← ADDED
  ├── component.go     (ComponentRef)
  ├── constraint.go    (Constraint)
  └── criteria.go      (Criteria)

pkg/snapshotter/
  ├── snapshot.go      (collection logic, imports types.Snapshot)
  └── agent.go
```

**Validation Controller usage:**
```go
// Clean single import
import "github.com/NVIDIA/aicr/pkg/types"

func (r *Reconciler) reconcile(ctx context.Context, cr *Validation) error {
    // Fetch snapshot
    snapshot := r.fetchSnapshot(ctx, cr.Spec.SnapshotRef)  // *types.Snapshot

    // Build validation
    validation := &cr.Spec.Validation  // types.Validation

    // Call AICR
    jobSpecs, err := aicr.BuildJobPlan(
        ctx,
        validation,  // *types.Validation
        snapshot,    // *types.Snapshot
        catalog,
    )
}
```

**Separate Snapshot CR:**

Snapshot is collected independently and referenced by Validation CR:

```yaml
# Snapshot CR (independent)
apiVersion: aicr.nvidia.com/v1alpha1
kind: Snapshot
metadata:
  name: cluster-snapshot-20260508
spec:
  refreshInterval: 1h  # Auto-refresh
  nodeSelector:
    nvidia.com/gpu.present: "true"
status:
  collectedAt: "2026-05-08T10:00:00Z"
  dataRef:
    name: snapshot-cm-20260508  # ConfigMap with snapshot data
  valid: true
---
# Validation CR (references Snapshot)
apiVersion: aicr.nvidia.com/v1alpha1
kind: Validation
metadata:
  name: validation-batch-1
spec:
  snapshotRef:
    name: cluster-snapshot-20260508  # Reference

  # Validation types (from pkg/types)
  deployment:
    checks: [operator-health]
  componentRefs:
    - name: gpu-operator
  criteria:
    service: eks
```

**Benefits:**
- ✅ Snapshot collected once, reused across validations
- ✅ Multiple Validations can share one Snapshot
- ✅ Fast validation reconciles (snapshot already exists)
- ✅ Historical validation (validate against old snapshots)
- ✅ Snapshot controller independent of recipe/validation

## Key Decisions Summary

| Decision | Rationale |
|----------|-----------|
| Create pkg/types package | Clean separation, both recipe and validator import shared types |
| Move Criteria, ComponentRef, Constraint | Used by both recipe and validation, should be neutral |
| **Validation as concrete struct** | **Simple design, no interface complexity** |
| Validation embeds Config + Context | Single struct for complete specification |
| ValidationContext = ComponentRefs + Criteria | Separates "what to run" from "what to validate" |
| Keep orchestration in controller | NodeSelector, ExpectedNewReplicas not part of AICR types |
| Recipe.ToValidation() conversion | Explicit conversion, breaking change acceptable pre-v1.0 |
| **Snapshot is recipe-independent** | **Snapshot collects actual state, no recipe dependency** |
| **Move Snapshot to pkg/types** | **Consistent with Validation, clean controller imports** |
| **Separate Snapshot CR** | **Collected once, reused across validations** |

## Benefits

1. **Clean architecture**: Portable types separate from domain logic
2. **External consumers**: Import just pkg/types, not pkg/recipe
3. **Evolution path**: RecipeResult → Validation for validation APIs
4. **CRD embedding**: Flat structure with inline embedding
5. **Future-proof**: Standalone validation YAML files possible
6. **Clear contracts**: Validation is self-contained specification

## Next Steps

1. Review this design document with team
2. Get approval for Option 3 scope expansion (including Snapshot)
3. Execute Phase 1 (create pkg/types, move 7 type groups)
4. Execute Phase 2 (update signatures for Validation + Snapshot)
5. Execute Phase 3 (tests + docs)
6. Close issue #732

## Discussion History

**Session 1 (2026-05-08):** Initial design exploration
- Analyzed issue #732 requirements
- Discovered validators need ComponentRefs + Criteria, not just ValidationConfig
- Explored package organization options (pkg/recipe vs pkg/validator vs pkg/types)
- **Decision:** Create pkg/types package with Validation parent type

**Session 2 (2026-05-08):** Snapshot dependency analysis
- Question: Does snapshot depend on recipe?
- **Finding:** Snapshot is completely recipe-independent (just observes cluster state)
- Question: Should controller import pkg/snapshotter or move Snapshot to pkg/types?
- **Decision:** Move Snapshot to pkg/types for consistency and clean controller imports
- **Decision:** Separate Snapshot CR (collected once, reused across validations)

**Session 3 (2026-05-08):** Recipe/Validation compatibility
- Question: Can Recipe be compatible with Validation to avoid breaking changes?
- Explored three approaches: interface, conversion helper, type alias
- Initially considered interface-based design for backward compatibility
- **User preference:** No interface-based approach
- **Decision:** Use explicit conversion with Recipe.ToValidation() method
- **Trade-off:** Breaking change (call sites must update), but acceptable pre-v1.0
- **Benefit:** Simple design - concrete struct, no interface complexity
- **Benefit:** Clear conversion point - explicit .ToValidation() calls

---

**Initial Design Date:** 2026-05-08
**Updated with Snapshot:** 2026-05-08
**Updated with Interface Design:** 2026-05-08
**Approved By:** [To be filled]
**Implementation Status:** Pending approval
