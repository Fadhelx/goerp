package agentdocscheck

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGoERPAgentSkillsHaveRequiredMetadata(t *testing.T) {
	root := repoRoot(t)
	for _, rel := range []string{
		"agent_skills/goerp-platform-kernel/SKILL.md",
		"agent_skills/goerp-web-theme/SKILL.md",
		"agent_skills/goerp-oi-parity/SKILL.md",
		"agent_skills/goerp-agent-orchestration/SKILL.md",
	} {
		text := readFile(t, filepath.Join(root, rel))
		if !strings.HasPrefix(text, "---\n") || !strings.Contains(text, "\nname: ") || !strings.Contains(text, "\ndescription: ") {
			t.Fatalf("%s missing skill frontmatter", rel)
		}
		for _, required := range []string{"Do not copy proprietary", "go test", "reports/progress_dashboard.html"} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s missing %q", rel, required)
			}
		}
	}
}

func TestEveBlueprintsUseAgentDirectoryContract(t *testing.T) {
	root := repoRoot(t)
	for _, agent := range []string{
		"agents/eve/goerp-platform-maintainer/agent",
		"agents/eve/goerp-ui-verifier/agent",
	} {
		instructions := readFile(t, filepath.Join(root, agent, "instructions.md"))
		if !strings.Contains(instructions, "# Identity") || !strings.Contains(instructions, "GoERP") {
			t.Fatalf("%s instructions missing identity", agent)
		}
		agentTS := readFile(t, filepath.Join(root, agent, "agent.ts"))
		if !strings.Contains(agentTS, `from "eve"`) || !strings.Contains(agentTS, "defineAgent") {
			t.Fatalf("%s agent.ts is not Eve-shaped", agent)
		}
		if entries := mustReadDir(t, filepath.Join(root, agent, "skills")); len(entries) == 0 {
			t.Fatalf("%s has no skills", agent)
		}
		if entries := mustReadDir(t, filepath.Join(root, agent, "subagents")); len(entries) == 0 {
			t.Fatalf("%s has no subagents", agent)
		}
		if entries := mustReadDir(t, filepath.Join(root, agent, "tools")); len(entries) == 0 {
			t.Fatalf("%s has no tools", agent)
		}
		if entries := mustReadDir(t, filepath.Join(root, agent, "sandbox")); len(entries) == 0 {
			t.Fatalf("%s has no sandbox config", agent)
		}
		if entries := mustReadDir(t, filepath.Join(root, agent, "schedules")); len(entries) == 0 {
			t.Fatalf("%s has no schedules", agent)
		}
	}
}

func TestAgentDocumentationIndexPointsToSkillsAndBlueprints(t *testing.T) {
	root := repoRoot(t)
	readme := readFile(t, filepath.Join(root, "README.md"))
	for _, required := range []string{"agent_skills/", "agents/eve/", "Eve-ready agent blueprints"} {
		if !strings.Contains(readme, required) {
			t.Fatalf("README missing %q", required)
		}
	}
	index := readFile(t, filepath.Join(root, "agents/eve/README.md"))
	for _, required := range []string{"instructions.md", "agent.ts", "skills/", "subagents/", "tools/", "sandbox/", "schedules/", "npx eve@latest init"} {
		if !strings.Contains(index, required) {
			t.Fatalf("Eve index missing %q", required)
		}
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func mustReadDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "../.."))
}
