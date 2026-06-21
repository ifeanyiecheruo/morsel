package cli

import (
	"strings"
	"testing"

	"github.com/ifeanyiecheruo/morsel/internal/platform"
)

func TestConsolePrompter_Ask_CollectsAnswers(t *testing.T) {
	prompts := []platform.Prompt{
		{Key: "name", Label: "Cluster name", Required: true},
		{Key: "region", Label: "Region", Default: "us-east-1"},
	}
	var out strings.Builder
	p := NewConsolePrompter(strings.NewReader("my-cluster\n\n"), &out)

	answers, err := p.Ask(prompts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if answers["name"] != "my-cluster" {
		t.Errorf("name: want %q, got %q", "my-cluster", answers["name"])
	}
	if answers["region"] != "us-east-1" {
		t.Errorf("region: want %q (default), got %q", "us-east-1", answers["region"])
	}
}

func TestConsolePrompter_Ask_RequiredFieldEmpty(t *testing.T) {
	prompts := []platform.Prompt{
		{Key: "name", Label: "Cluster name", Required: true},
	}
	var out strings.Builder
	p := NewConsolePrompter(strings.NewReader("\n"), &out)

	_, err := p.Ask(prompts)
	if err == nil {
		t.Fatal("expected error for empty required field, got nil")
	}
}

func TestConsolePrompter_Ask_DefaultShownInPrompt(t *testing.T) {
	prompts := []platform.Prompt{
		{Key: "region", Label: "Region", Default: "eu-west-1"},
	}
	var out strings.Builder
	p := NewConsolePrompter(strings.NewReader("\n"), &out)

	if _, err := p.Ask(prompts); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out.String(), "eu-west-1") {
		t.Errorf("expected default value in prompt output, got: %q", out.String())
	}
}

func TestConsolePrompter_PrintPlan_IncludesSummaryAndResources(t *testing.T) {
	plan := platform.Plan{
		Summary: "Create 2 resources",
		Resources: []platform.Resource{
			{Name: "namespace", Description: "Kubernetes namespace"},
			{Name: "secret", Description: "Signing key secret"},
		},
	}
	var out strings.Builder
	p := NewConsolePrompter(strings.NewReader(""), &out)
	p.PrintPlan(plan)

	got := out.String()
	if !strings.Contains(got, "Create 2 resources") {
		t.Errorf("summary missing from output: %q", got)
	}
	if !strings.Contains(got, "namespace") {
		t.Errorf("resource name missing from output: %q", got)
	}
	if !strings.Contains(got, "Kubernetes namespace") {
		t.Errorf("resource description missing from output: %q", got)
	}
}

func TestConsolePrompter_Confirm_AcceptsY(t *testing.T) {
	for _, input := range []string{"y\n", "Y\n", "yes\n", "YES\n"} {
		var out strings.Builder
		p := NewConsolePrompter(strings.NewReader(input), &out)
		if !p.Confirm("Proceed? [y/N]: ") {
			t.Errorf("expected true for input %q", input)
		}
	}
}

func TestConsolePrompter_Confirm_RejectsOther(t *testing.T) {
	for _, input := range []string{"n\n", "\n", "no\n", "maybe\n"} {
		var out strings.Builder
		p := NewConsolePrompter(strings.NewReader(input), &out)
		if p.Confirm("Proceed? [y/N]: ") {
			t.Errorf("expected false for input %q", input)
		}
	}
}
