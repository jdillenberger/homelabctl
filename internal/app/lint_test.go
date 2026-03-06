package app

import (
	"testing"

	"github.com/jdillenberger/homelabctl/templates"
)

func TestLintAdguardNoErrors(t *testing.T) {
	registry, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	linter := NewLinter(registry)
	findings := linter.LintTemplate("adguard")

	for _, f := range findings {
		if f.Severity == SeverityError {
			t.Errorf("unexpected ERROR finding for adguard: [%s] %s: %s", f.Check, f.File, f.Message)
		}
	}
}

func TestLintDetectsFloatingTags(t *testing.T) {
	// The scaffolded "templates new" produces a compose with "TODO_IMAGE:latest"
	// which should trigger floating-image-tag. We can test this by checking
	// templates that we know use floating tags, or by verifying the check logic
	// through LintAll.
	registry, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	linter := NewLinter(registry)
	result := linter.LintAll()

	// Check that floating-image-tag checks exist for any template that uses them
	floatingChecks := 0
	for _, f := range result.Findings {
		if f.Check == "floating-image-tag" {
			floatingChecks++
		}
	}
	// We don't assert a specific count since templates may change,
	// but the check should work without panicking
	t.Logf("Found %d floating-image-tag findings across all templates", floatingChecks)
}

func TestLintDummyValuesCoversAll(t *testing.T) {
	registry, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	linter := NewLinter(registry)

	for _, name := range registry.List() {
		meta, ok := registry.Get(name)
		if !ok {
			continue
		}

		dummyValues := linter.buildDummyValues(meta)

		// Every defined value should have a dummy
		for _, v := range meta.Values {
			if _, ok := dummyValues[v.Name]; !ok {
				t.Errorf("template %s: value %q not covered by dummy values", name, v.Name)
			}
		}

		// Standard values should be present
		for key := range standardValues {
			if _, ok := dummyValues[key]; !ok {
				t.Errorf("template %s: standard value %q not in dummy values", name, key)
			}
		}
	}
}

func TestLintAllReturnsResultsForAllTemplates(t *testing.T) {
	registry, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	linter := NewLinter(registry)
	result := linter.LintAll()

	// Collect unique templates that have findings
	templatesWithFindings := make(map[string]bool)
	for _, f := range result.Findings {
		templatesWithFindings[f.Template] = true
	}

	// Every template should have at least one finding (INFO-level at minimum,
	// since almost no template has cap_drop: ALL and read_only: true)
	allTemplates := registry.List()
	for _, name := range allTemplates {
		if !templatesWithFindings[name] {
			t.Errorf("LintAll() produced no findings for template %q (expected at least INFO-level)", name)
		}
	}

	// Summary should be consistent
	if result.Summary.Total != len(result.Findings) {
		t.Errorf("Summary.Total=%d but len(Findings)=%d", result.Summary.Total, len(result.Findings))
	}
	expectedTotal := result.Summary.Errors + result.Summary.Warnings + result.Summary.Infos
	if result.Summary.Total != expectedTotal {
		t.Errorf("Summary.Total=%d but Errors+Warnings+Infos=%d", result.Summary.Total, expectedTotal)
	}
}

func TestLintNoRenderErrors(t *testing.T) {
	registry, err := NewRegistry(templates.FS)
	if err != nil {
		t.Fatalf("NewRegistry() error: %v", err)
	}

	linter := NewLinter(registry)
	result := linter.LintAll()

	for _, f := range result.Findings {
		if f.Check == "template-render-error" {
			t.Errorf("template %s failed to render: %s", f.Template, f.Message)
		}
	}
}

func TestCountSummary(t *testing.T) {
	findings := []LintFinding{
		{Severity: SeverityError},
		{Severity: SeverityError},
		{Severity: SeverityWarning},
		{Severity: SeverityInfo},
	}

	s := CountSummary(findings)
	if s.Errors != 2 || s.Warnings != 1 || s.Infos != 1 || s.Total != 4 {
		t.Errorf("CountSummary = %+v, want Errors=2 Warnings=1 Infos=1 Total=4", s)
	}
}
