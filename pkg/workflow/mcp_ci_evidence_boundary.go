package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

// CIEvidenceBoundaryConfig selects an independently reviewed immutable gateway
// artifact. No default artifact exists: source preparation is not activation.
type CIEvidenceBoundaryConfig struct {
	Image string `yaml:"image"`
}

var ciEvidenceImagePattern = regexp.MustCompile(`^ghcr\.io/[a-z0-9][a-z0-9/_.-]*@sha256:[a-f0-9]{64}$`)

func validateCIEvidenceBoundary(cfg *MCPGatewayRuntimeConfig, tools map[string]any) error {
	if cfg.CIEvidenceBoundary == nil {
		return nil
	}
	if !ciEvidenceImagePattern.MatchString(cfg.CIEvidenceBoundary.Image) {
		return fmt.Errorf("ci-evidence-boundary requires an explicit immutable ghcr.io image digest")
	}
	if _, ok := tools["ci-evidence"]; !ok {
		return fmt.Errorf("ci-evidence-boundary requires ci-evidence backend")
	}
	if cfg.Entrypoint != "" || len(cfg.Args) > 0 || len(cfg.EntrypointArgs) > 0 {
		return fmt.Errorf("ci-evidence-boundary forbids gateway executable overrides")
	}
	for name := range cfg.Env {
		if name == "MA_CI_EVIDENCE_LEASE" || name == "GH_AW_CI_EVIDENCE_STATE_DIR" {
			return fmt.Errorf("ci-evidence-boundary reserves state environment")
		}
	}
	for _, mount := range cfg.Mounts {
		parts := strings.Split(mount, ":")
		if len(parts) != 3 || parts[1] == "/" || strings.HasPrefix(parts[1], "/private-gateway-state") || strings.HasPrefix(parts[0], "/run/gh-aw-ci-evidence-") {
			return fmt.Errorf("ci-evidence-boundary forbids conflicting state mounts")
		}
	}
	return nil
}

func writeCIEvidenceStateSetup(yaml *strings.Builder) {
	// GitHub's fixed runner identity fields cannot introduce shell metacharacters.
	// mkdir (not mkdir -p) refuses stale state; restarting the container retains
	// this exact private mount. No state path is written to GITHUB_ENV or artifacts.
	yaml.WriteString("          [[ \"$GITHUB_RUN_ID\" =~ ^[0-9]+$ && \"$GITHUB_RUN_ATTEMPT\" =~ ^[0-9]+$ && \"$GITHUB_JOB\" =~ ^[A-Za-z_][A-Za-z0-9_-]*$ ]] || exit 1\n")
	yaml.WriteString("          GH_AW_CI_EVIDENCE_STATE_DIR=\"/run/gh-aw-ci-evidence-${GITHUB_RUN_ID}-${GITHUB_RUN_ATTEMPT}-${GITHUB_JOB}\"\n")
	yaml.WriteString("          sudo mkdir -m 0700 \"$GH_AW_CI_EVIDENCE_STATE_DIR\"\n")
	yaml.WriteString("          sudo chown \"$(id -u):$(id -g)\" \"$GH_AW_CI_EVIDENCE_STATE_DIR\"\n")
	yaml.WriteString("          export MA_CI_EVIDENCE_LEASE=/private-gateway-state/session.json\n")
}
