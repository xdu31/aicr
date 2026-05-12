# API Versioning for ValidationInput Breaking Changes

## Problem

When AICR makes breaking changes (e.g., v1alpha1 → v1alpha2), external controllers need both versions simultaneously to implement Kubernetes CRD conversion webhooks. However, Go modules cannot have two minor versions of the same module path coexist in go.mod.

## Three Options

**1. Go Module Major Versions**
- Breaking change triggers v2.0.0 with module path `github.com/NVIDIA/aicr/v2`
- Controllers import both `github.com/NVIDIA/aicr` and `github.com/NVIDIA/aicr/v2`

**2. Fork Repository**
- Create `github.com/NVIDIA/aicr-v1` fork for old version
- Controllers import both `github.com/NVIDIA/aicr-v1` and `github.com/NVIDIA/aicr`

**3. Controller Copies Types**
- Controller copies v1alpha1 types locally before upgrading AICR
- AICR makes breaking changes in minor versions

## Decision: Option 1 (Major Versions)

**Agreed with @mchmarny:** Once at v1, breaking API changes require major version bumps.

Standard Go practice for breaking changes, semantic versioning compliant, and maintains one repository.

### Policy
- **Before v1 (v1alpha1):** Minimal changes expected in short window to v1
- **v1 and beyond:** Breaking changes require major version bump (v2.0.0 with `/v2` module path)
