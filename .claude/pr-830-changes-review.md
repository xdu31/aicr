# PR #830 Changes Review - Addressing Mark's Comments

## Overview
Refactor Validation type extraction to address Mark's 4 main concerns:
- (a) Package location
- (b) API group
- (c) Type naming
- (d) Inline embed issue

## 1. Package Structure Changes

### Before
```
pkg/recipe/
├── validation.go          # Validation type
└── validation_test.go
```

### After
```
pkg/api/validator/v1/
├── validation_input.go       # ValidationInput type
├── validation_input_test.go
└── doc.go                    # Stability disclaimer

pkg/recipe/
├── recipe.go                 # ToValidationInput() function moves here
├── component.go              # ComponentRef (referenced by ValidationInput)
├── criteria.go               # Criteria (referenced by ValidationInput)
├── constraint.go             # Constraint (referenced by ValidationInput)
└── ... (other recipe types)
```

## 2. Type Structure Changes

### Before (pkg/recipe/validation.go)
```go
type Validation struct {
    APIVersion string              `json:"apiVersion,omitempty"`
    Kind       string              `json:"kind,omitempty"`
    Metadata   *ValidationMetadata `json:"metadata,omitempty"`

    ValidationConfig `json:",inline" yaml:",inline"`  // ❌ Inline embed

    ComponentRefs []ComponentRef `json:"componentRefs,omitempty"`
    Criteria      Criteria       `json:"criteria,omitempty"`
    Constraints   []Constraint   `json:"constraints,omitempty"`
}

type ValidationConfig struct {
    Readiness   *ValidationPhase
    Deployment  *ValidationPhase
    Performance *ValidationPhase
    Conformance *ValidationPhase
}

const KindValidation = "Validation"
```

### After (pkg/api/validator/v1/validation_input.go)
```go
package v1

import "github.com/NVIDIA/aicr/pkg/recipe"

type ValidationInput struct {
    APIVersion string              `json:"apiVersion,omitempty" yaml:"apiVersion,omitempty"`
    Kind       string              `json:"kind,omitempty" yaml:"kind,omitempty"`
    Metadata   *ValidationMetadata `json:"metadata,omitempty" yaml:"metadata,omitempty"`

    Spec ValidationSpec `json:"spec" yaml:"spec"`  // ✅ Named field

    ComponentRefs []recipe.ComponentRef `json:"componentRefs,omitempty" yaml:"componentRefs,omitempty"`
    Criteria      recipe.Criteria       `json:"criteria,omitempty" yaml:"criteria,omitempty"`
    Constraints   []recipe.Constraint   `json:"constraints,omitempty" yaml:"constraints,omitempty"`
}

type ValidationMetadata struct {
    Name    string `json:"name,omitempty" yaml:"name,omitempty"`
    Version string `json:"version,omitempty" yaml:"version,omitempty"`
}

type ValidationSpec struct {  // ✅ Renamed from ValidationConfig
    Readiness   *ValidationPhase `json:"readiness,omitempty" yaml:"readiness,omitempty"`
    Deployment  *ValidationPhase `json:"deployment,omitempty" yaml:"deployment,omitempty"`
    Performance *ValidationPhase `json:"performance,omitempty" yaml:"performance,omitempty"`
    Conformance *ValidationPhase `json:"conformance,omitempty" yaml:"conformance,omitempty"`
}

type ValidationPhase struct {
    Timeout        string               `json:"timeout,omitempty" yaml:"timeout,omitempty"`
    Constraints    []recipe.Constraint  `json:"constraints,omitempty" yaml:"constraints,omitempty"`
    Checks         []string             `json:"checks,omitempty" yaml:"checks,omitempty"`
    NodeSelection  *NodeSelection       `json:"nodeSelection,omitempty" yaml:"nodeSelection,omitempty"`
    Infrastructure string               `json:"infrastructure,omitempty" yaml:"infrastructure,omitempty"`
}

type NodeSelection struct {
    Selector     map[string]string `json:"selector,omitempty" yaml:"selector,omitempty"`
    MaxNodes     int               `json:"maxNodes,omitempty" yaml:"maxNodes,omitempty"`
    ExcludeNodes []string          `json:"excludeNodes,omitempty" yaml:"excludeNodes,omitempty"`
}

const KindValidationInput = "ValidationInput"  // ✅ Renamed
```

## 3. API Version Changes

### Before
```yaml
apiVersion: aicr.nvidia.com/v1
kind: Validation
```

### After
```yaml
apiVersion: validator.nvidia.com/v1alpha1
kind: ValidationInput
```

## 4. YAML Structure Changes

### Before (Inline Embed - Flat Structure)
```yaml
apiVersion: aicr.nvidia.com/v1
kind: Validation
metadata:
  name: my-validation
readiness:          # ← Phase fields at top level
  timeout: 10m
deployment:
  timeout: 20m
componentRefs:
  - name: gpu-operator
```

### After (Named Spec Field - Nested Structure)
```yaml
apiVersion: validator.nvidia.com/v1alpha1
kind: ValidationInput
metadata:
  name: my-validation
spec:               # ← Phases nested under spec
  readiness:
    timeout: 10m
  deployment:
    timeout: 20m
componentRefs:
  - name: gpu-operator
```

## 5. Function Renames

| Before | After |
|--------|-------|
| `ToValidation(*RecipeResult) *Validation` | `ToValidationInput(*RecipeResult) *v1.ValidationInput` |
| `NewValidation() *Validation` | `NewValidationInput() *v1.ValidationInput` |

## 6. Package Documentation (New File)

**pkg/api/validator/v1/doc.go:**
```go
// Package v1 defines AICR's validator input format (v1alpha1).
//
// # Stability
//
// v1alpha1 is unstable and may have breaking changes before v1.
// Breaking changes at v1+ will require major version bumps (v2.0.0).
//
// # API Group
//
// validator.nvidia.com is a non-binding example.
// AICR ships no CRDs - external projects should use their own API groups.
//
// # Usage
//
// This package defines ValidationInput, the input format for AICR's
// validator plugins. It carries both validation spec (phases, checks)
// and recipe context (ComponentRefs, Criteria, Constraints).
package v1
```

## 7. Import Changes Required

### Files Needing Import Updates (~20-25 files)

**Core packages:**
- `pkg/recipe/recipe.go` - has ToValidationInput function
- `pkg/validator/validator.go` - main validator logic
- `pkg/validator/job/deployer.go` - creates validation Jobs
- `pkg/validator/catalog/catalog.go` - validator catalog

**All validators (validators/):**
- `validators/context.go` - validator Context struct
- `validators/deployment/*.go` - deployment validators
- `validators/performance/*.go` - performance validators
- `validators/conformance/*.go` - conformance validators
- `validators/readiness/*.go` - readiness validators

**Tests:**
- All test files that use Validation type

**Import change pattern:**
```diff
- import "github.com/NVIDIA/aicr/pkg/recipe"
+ import (
+     "github.com/NVIDIA/aicr/pkg/recipe"
+     v1 "github.com/NVIDIA/aicr/pkg/api/validator/v1"
+ )

- validation := &recipe.Validation{}
+ validation := &v1.ValidationInput{}
```

## 8. Field Access Changes

### Before
```go
// Inline embed - fields at top level
ctx.Recipe.Readiness.Timeout
ctx.Recipe.Performance.Checks
```

### After
```go
// Named spec field - fields under Spec
ctx.ValidationInput.Spec.Readiness.Timeout
ctx.ValidationInput.Spec.Performance.Checks
```

## 9. PR Description Update

Remove this line:
```markdown
🤖 Generated with [Claude Code](https://claude.com/claude-code)
```

## 10. Estimated Impact

| Category | Files Changed | Complexity |
|----------|---------------|------------|
| New package creation | 3 files | Medium |
| Type refactoring | 1 file | High |
| Import updates | ~20-25 files | Medium |
| Field access updates | ~15 files | Low |
| Test updates | ~5 files | Medium |
| **Total** | **~30 files** | **High** |

## 11. Breaking Changes

**For AICR internal:**
- All validator implementations need updates
- All tests need updates
- Field access patterns change (`.Readiness` → `.Spec.Readiness`)

**For external consumers (hypothetical):**
- Import path changes: `pkg/recipe` → `pkg/api/validator/v1`
- Type name changes: `Validation` → `ValidationInput`
- YAML structure changes: flat → nested under `spec:`
- API version changes: `aicr.nvidia.com/v1` → `validator.nvidia.com/v1alpha1`

## 12. Testing Strategy

1. **Unit tests** - All existing tests should pass with updated types
2. **Integration tests** - E2E validator tests should work with new structure
3. **YAML parsing** - Verify both old and new YAML formats (if backwards compat needed)
4. **Import verification** - Ensure no broken imports remain

## 13. Risk Assessment

| Risk | Mitigation |
|------|------------|
| Missed import updates | Run `make test` and `make lint` after each phase |
| Field access errors | Systematic search/replace for `.Readiness`, `.Performance`, etc. |
| Test failures | Update tests incrementally with code changes |
| Merge conflicts | Already rebased on latest main |

## 14. Decision Summary

✅ **Agreed with Mark:**
- (a) Move to pkg/api/validator/v1
- (b) Use validator.nvidia.com/v1alpha1 with disclaimer
- (c) Rename to ValidationInput (keep recipe fields)
- (d) Use named Spec field (not inline embed)
- Keep ValidationMetadata (Option A)
- Major version bumps for breaking changes at v1+

## 15. Implementation Order

1. Create new package structure
2. Implement new types in pkg/api/validator/v1
3. Update ToValidationInput in pkg/recipe
4. Update pkg/validator imports and usage
5. Update validators/ imports and field access
6. Update all tests
7. Remove old pkg/recipe/validation.go
8. Run make test and make lint
9. Update PR description
10. Commit and force push
