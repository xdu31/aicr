# Issue #764 Analysis: AWS EFA Region Hardcoding

**Issue URL**: https://github.com/NVIDIA/aicr/issues/764
**Status**: Open
**Labels**: area/recipes, area/bundler, bug

## Problem Summary

The AWS EFA component hardcodes `us-west-2` in its ECR image repository URL, causing issues for clusters in other AWS regions.

**Current configuration** (`recipes/components/aws-efa/values.yaml:19`):
```yaml
image:
  repository: 602401143452.dkr.ecr.us-west-2.amazonaws.com/eks/aws-efa-k8s-device-plugin
  tag: v0.5.3
```

## Why This Matters

AWS publishes EKS add-on images regionally in private regional ECR repositories for:
1. **Network isolation** - pulls over AWS internal backbone
2. **Cost & rate limits** - avoids Docker Hub limits and NAT Gateway charges
3. **Availability** - image lives in same region as cluster

Non-us-west-2 clusters either pay cross-region NAT egress or fail if cross-region access is blocked.

## Current AICR Mechanisms

### Existing `--dynamic` Mechanism
Declares paths that should be provided at helm **install time** (not bundle time):
- Usage: `aicr bundle --dynamic awsefa:image.repository`
- Effect: Moves that path from `values.yaml` to `cluster-values.yaml`
- Install: `helm install --set awsefa.image.repository=<full-url>`
- Files: `pkg/bundler/config/config.go` (lines 129-131), `pkg/bundler/deployer/localformat/writer.go` (line 276)

### Existing `--set` Mechanism
Applies value overrides at bundle time:
- Usage: `aicr bundle --set awsefa:image.repository=<full-url>`
- Effect: Bakes the value into the bundle's `values.yaml`
- Files: `pkg/bundler/bundler.go` (lines 461-480)

## Proposed Implementation Approaches

### Option 1: Bundle-Time Templating (Region-Specific Bundles)
Add Go template processing to values.yaml files during bundle generation.

**Implementation**:
- Add new flag: `--region <region>` or reuse `--set awsefa:region=us-east-1`
- Process values.yaml as Go template before loading
- Template syntax in values.yaml: `repository: 602401143452.dkr.ecr.{{ .region }}.amazonaws.com/...`
- Default region: `us-east-1` (if not specified)

**Pros**: Simple user experience, correct region baked into bundle
**Cons**: Need separate bundles per region, requires template processing infrastructure

### Option 2: Install-Time Dynamic Value (Region-Agnostic Bundles)
Use existing `--dynamic` mechanism.

**Implementation**:
- No code changes needed
- Usage: `aicr bundle --dynamic awsefa:image.repository`
- Install: `helm install --set awsefa.image.repository=602401143452.dkr.ecr.us-west-2.amazonaws.com/...`

**Pros**: No code changes, bundle works for all regions
**Cons**: Verbose for users (must construct full URL), easy to make mistakes

### Option 3: Structured Dynamic Value with Helper
Add region-aware templating at install time.

**Implementation**:
- Document pattern for users to provide just the region
- Possibly add Helm template helpers to construct the URL
- Usage: `helm install --set awsefa.region=us-west-2`
- Helm template in chart constructs full URL

**Pros**: Flexible, region-agnostic bundles, simpler user input
**Cons**: Requires Helm template logic, documentation updates

## Key Files for Implementation

- `recipes/components/aws-efa/values.yaml` - current hardcoded value
- `recipes/registry.yaml` (lines 132-149) - aws-efa component config
- `pkg/bundler/bundler.go` (line 243) - `extractComponentValues()` entry point
- `pkg/recipe/result.go` - `GetValuesForComponent()` (loads values.yaml)
- `pkg/bundler/deployer/localformat/writer.go` (line 275) - writes values files
- `docs/user/cli-reference.md` - CLI documentation for `--dynamic` flag
- `docs/integrator/recipe-development.md` - recipe development guide

## Out of Scope (Noted in Issue)

- GovCloud / China partition support (different account IDs and URI suffixes)
- Audit of other recipes for similar regional hardcoding
- Public ECR availability check (`public.ecr.aws/eks/aws-efa-k8s-device-plugin`)

## Acceptance Criteria

- [ ] `recipes/components/aws-efa/values.yaml` no longer hardcodes a region
- [ ] Default bundle produces valid image URI for chosen default region
- [ ] Mechanism to specify custom region (via flag or override)
- [ ] Committed BOM (`docs/user/container-images.md`) reflects templated form with explanatory note
- [ ] Test in `pkg/bundler` verifies templated value resolves correctly (default + override)
- [ ] Recipe development docs document the region mechanism
- [ ] EKS guide updated with region guidance

## Next Steps

**Decision needed**: Which implementation approach (Option 1, 2, 3, or hybrid)?

Once approach is decided:
1. Read `pkg/recipe/result.go` to understand `GetValuesForComponent()`
2. Implement template processing or dynamic value mechanism
3. Add tests for region substitution
4. Update documentation
5. Update BOM generation
