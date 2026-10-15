package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLoadSkills(t *testing.T) {
	skills, err := LoadSkills([]string{"testdata/skills/single", "testdata/skills/pack", "testdata/skills/folded"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, s := range skills {
		got[s.Name] = s.Description
	}
	want := map[string]string{
		"single":            "Use when writing Go. Quoted, with a colon: like this.",
		"plan":              "Use when you have a spec, before touching code",
		"using-superpowers": "Use when starting any conversation",
		"folded":            "Terse voice: answer first. Use when asked to be brief.",
		// "manual" is disable-model-invocation; "fake" sits under tests/.
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("skills = %v\nwant %v", got, want)
	}
}

func TestLoadSkillsErrors(t *testing.T) {
	if _, err := LoadSkills([]string{t.TempDir()}); err == nil {
		t.Error("a folder with no SKILL.md should be an error")
	}
	if _, err := LoadSkills([]string{"testdata/skills/single", "testdata/skills/single"}); err == nil {
		t.Error("the same skill twice should be an error")
	}
}

func TestSkillsPrompt(t *testing.T) {
	skills, err := LoadSkills([]string{"testdata/skills/pack"})
	if err != nil {
		t.Fatal(err)
	}
	prompt, err := skillsPrompt(skills)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"The following skills are available for use with the Skill tool:",
		"- plan: Use when you have a spec, before touching code",
		"<EXTREMELY_IMPORTANT>\nYou have superpowers.", // the pack's session-start text, as its hook adds it
		"Always check for a skill first.",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("prompt is missing %q:\n%s", want, prompt)
		}
	}
	if p, _ := skillsPrompt(nil); p != "" {
		t.Errorf("no skills should add nothing, got %q", p)
	}
}

func TestLoadSkillTool(t *testing.T) {
	skills, err := LoadSkills([]string{"testdata/skills/folded", "testdata/skills/pack"})
	if err != nil {
		t.Fatal(err)
	}
	out, err := loadSkill(skills, "folded", "")
	if err != nil || !strings.Contains(out, "# Folded") || !strings.Contains(out, "references/testing.md") {
		t.Errorf("SKILL.md load = %q, %v (want the content and the list of its other files)", out, err)
	}
	if out, err := loadSkill(skills, "folded", "references/testing.md"); err != nil || !strings.Contains(out, "Testing reference") {
		t.Errorf("reference file = %q, %v", out, err)
	}
	if _, err := loadSkill(skills, "superpowers:plan", ""); err != nil {
		t.Errorf("the superpowers: prefix should resolve: %v", err)
	}
	for _, bad := range []string{"../../outside.txt", "/etc/passwd"} {
		if _, err := loadSkill(skills, "folded", bad); err == nil {
			t.Errorf("file %q outside the skill folder was allowed", bad)
		}
	}
	if _, err := loadSkill(skills, "nope", ""); err == nil || !strings.Contains(err.Error(), "installed: folded") {
		t.Errorf("unknown skill error should list what's installed, got %v", err)
	}
}

func toolNames(tb *Toolbox) string { return strings.Join(tb.ToolNames(), ",") }

func TestToolboxOffersEp04ToolsOnlyWhenAsked(t *testing.T) {
	tb, err := NewToolbox(t.TempDir(), &LocalSandbox{Timeout: time.Minute}, 20000)
	if err != nil {
		t.Fatal(err)
	}
	if got := toolNames(tb); got != "list_files,read_file,write_file,go_build,go_test,go_vet" {
		t.Errorf("the default toolbox changed: %s (Ep02 runs must stay comparable)", got)
	}
	tb.skills, _ = LoadSkills([]string{"testdata/skills/single"})
	tb.reviewer = &Reviewer{}
	if got := toolNames(tb); !strings.HasSuffix(got, ",Skill,request_review") {
		t.Errorf("tools = %s, want Skill and request_review added", got)
	}
	ro := tb.ReadOnly()
	if got := toolNames(ro); got != "list_files,read_file,go_build,go_test,go_vet" {
		t.Errorf("read-only tools = %s", got)
	}
	res := ro.Run(context.Background(), call("w", "write_file", map[string]any{"path": "x.go", "content": "package x"}))
	if !res.IsError {
		t.Error("the read-only reviewer was able to write a file")
	}
	if res := ro.Run(context.Background(), call("s", "Skill", map[string]any{"skill": "single"})); !res.IsError {
		t.Error("the reviewer was able to load a skill")
	}
}

// TestRequestReview: the contestant asks for a review; a separate agent reads the code and
// replies; the reviewer's tokens are counted.
func TestRequestReview(t *testing.T) {
	reviewModel := &scriptedModel{replies: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{call("l", "list_files", map[string]any{"path": "."})}},
		{Role: "assistant", Text: "### Error handling\n- none found"},
	}}
	agent, workspace, _ := newTestAgent(t, &scriptedModel{replies: []Message{
		{Role: "assistant", ToolCalls: []ToolCall{call("r", "request_review", nil)}},
		{Role: "assistant", Text: "DONE"},
	}}, 5, time.Now().Add(time.Minute))
	if err := os.WriteFile(filepath.Join(workspace, "main.go"), []byte("package main\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	agent.Tools.reviewer = &Reviewer{Model: reviewModel, Tools: agent.Tools.ReadOnly(), Spec: "the spec"}
	result := agent.Run(context.Background(), "build it")
	if result.StoppedBy != StoppedDone || result.ToolsByName["request_review"] != 1 {
		t.Fatalf("result = %+v", result)
	}
	r := agent.Tools.reviewer
	if r.Calls != 1 || r.Usage.InputTokens != 2000 {
		t.Errorf("reviewer calls %d, usage %+v (want 1 call, 2 model calls of 1000 input tokens)", r.Calls, r.Usage)
	}
	// A review that never finishes is reported to the contestant as a tool error.
	stuck := &Reviewer{Model: &scriptedModel{}, Tools: agent.Tools.ReadOnly()}
	if _, err := stuck.Review(context.Background()); err == nil {
		t.Error("a reviewer that writes no review should be an error")
	}
}

func TestStatsRecordSkills(t *testing.T) {
	data, _ := json.Marshal(Stats{Skills: []string{"tdd"}, ReviewCalls: 2})
	if !strings.Contains(string(data), `"skills":["tdd"]`) || !strings.Contains(string(data), `"review_calls":2`) {
		t.Errorf("stats JSON = %s", data)
	}
	data, _ = json.Marshal(Stats{})
	if strings.Contains(string(data), "skills") || strings.Contains(string(data), "review_calls") {
		t.Errorf("a run without skills should keep the Ep02 stats shape: %s", data)
	}
}
