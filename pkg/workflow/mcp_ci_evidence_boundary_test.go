package workflow

import (
	"strings"
	"testing"
)

// Deliberately synthetic, never a published artifact or activation pin.
const syntheticEvidenceImage = "ghcr.io/example/ci-evidence-gateway@sha256:" + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestCIEvidenceBoundaryRejectsUnpinnedArtifact(t *testing.T) {
	for _, image := range []string{"", "ghcr.io/example/gateway:latest", "${{ inputs.image }}", "x@sha256:bad"} {
		cfg := &MCPGatewayRuntimeConfig{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: image}}
		if validateCIEvidenceBoundary(cfg, map[string]any{"ci-evidence": map[string]any{}}) == nil {
			t.Fatalf("accepted %q", image)
		}
	}
}
func TestCIEvidenceBoundaryRefusesConflictingAuthority(t *testing.T) {
	for _, cfg := range []*MCPGatewayRuntimeConfig{
		{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: syntheticEvidenceImage}, Env: map[string]string{"MA_CI_EVIDENCE_LEASE": "/tmp/reset"}},
		{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: syntheticEvidenceImage}, Mounts: []string{"/tmp:/private-gateway-state:rw"}},
		{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: syntheticEvidenceImage}, Entrypoint: "/bin/sh"},
	} {
		if validateCIEvidenceBoundary(cfg, map[string]any{"ci-evidence": map[string]any{}}) == nil {
			t.Fatal("accepted conflicting authority")
		}
	}
	cfg := &MCPGatewayRuntimeConfig{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: syntheticEvidenceImage}}
	if validateCIEvidenceBoundary(cfg, map[string]any{}) == nil {
		t.Fatal("accepted absent backend")
	}
	if err := validateCIEvidenceBoundary(cfg, map[string]any{"ci-evidence": map[string]any{}}); err != nil {
		t.Fatal(err)
	}
}
func TestCIEvidenceBoundaryPrivateState(t *testing.T) {
	var b strings.Builder
	writeCIEvidenceStateSetup(&b)
	for _, required := range []string{"mkdir -m 0700", "chown", "GITHUB_RUN_ID", "GITHUB_RUN_ATTEMPT", "GITHUB_JOB", "MA_CI_EVIDENCE_LEASE=/private-gateway-state/session.json"} {
		if !strings.Contains(b.String(), required) {
			t.Fatalf("missing %s", required)
		}
	}
	if strings.Contains(b.String(), "mkdir -p") || strings.Contains(b.String(), "/tmp/gh-aw") {
		t.Fatal("state must refuse reuse and avoid shared agent directory")
	}
}

func TestCIEvidenceBoundaryRender(t *testing.T) {
	tools := map[string]any{"ci-evidence": map[string]any{"container": "example.invalid/reader", "entrypoint": "node", "args": []string{"/reader.mjs"}}}
	data := &WorkflowData{Tools: tools, EngineConfig: &EngineConfig{ID: "copilot"}, SandboxConfig: &SandboxConfig{MCP: &MCPGatewayRuntimeConfig{CIEvidenceBoundary: &CIEvidenceBoundaryConfig{Image: syntheticEvidenceImage}}}}
	var out strings.Builder
	if err := generateMCPGatewaySetup(&out, tools, []string{"ci-evidence"}, NewCopilotEngine(), data, false, nil); err != nil {
		t.Fatal(err)
	}
	for _, part := range []string{syntheticEvidenceImage, "--pull=never", "-e MA_CI_EVIDENCE_LEASE", "/private-gateway-state:rw", "sudo mkdir -m 0700"} {
		if !strings.Contains(out.String(), part) {
			t.Fatalf("missing %q", part)
		}
	}
	if strings.Contains(out.String(), syntheticEvidenceImage+":") {
		t.Fatal("digest retagged")
	}
	if strings.Contains(buildMCPGatewayAllowedMountRoots(tools, data.SandboxConfig.MCP), "private-gateway-state") {
		t.Fatal("gateway private state exposed to backend mount allowlist")
	}
	cfg := (&Compiler{}).extractMCPGatewayConfig(map[string]any{"ci-evidence-boundary": map[string]any{"image": syntheticEvidenceImage}})
	if cfg.CIEvidenceBoundary == nil || cfg.CIEvidenceBoundary.Image != syntheticEvidenceImage {
		t.Fatal("public config discarded")
	}
}
