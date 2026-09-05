package prompts

import (
	"strings"
	"testing"

	"gogitor/internal/domain"
)

func TestCodeCreate(t *testing.T) {
	p := CodeCreate("create hello world", "")
	for _, kw := range []string{"senior Go engineer", "create hello world", "--- File:"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestCodeCreate_FixKeyword(t *testing.T) {
	p := CodeCreate("fix the error", "")

	if !strings.Contains(p, "--- File:") {
		t.Error("expected file-block output format")
	}

	if strings.Contains(p, "MODE: PATCH") {
		t.Error("CodeCreate must not use patch mode")
	}

	if !strings.Contains(p, "Do not use markdown code fences") {
		t.Error("expected no-markdown rule")
	}
}

func TestCodeCreate_WithContext(t *testing.T) {
	if p := CodeCreate("add function", "package main"); !strings.Contains(p, "PROJECT CONTEXT") {
		t.Error("missing context")
	}
}

func TestCodeModify(t *testing.T) {
	p := CodeModify("refactor", "package main")
	for _, kw := range []string{"source of truth", "refactor"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestCodeFix(t *testing.T) {
	p := CodeFix("fix", "package main", "undefined: foo")
	for _, kw := range []string{"ERRORS", "undefined: foo"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestAnalyze(t *testing.T) {
	p := Analyze("find bugs", "package main")
	for _, kw := range []string{"Analyze", "Do not modify files"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestChat(t *testing.T) {
	p := Chat("history", "what is Go?")
	for _, kw := range []string{"Gogitor", "what is Go?"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestPlan(t *testing.T) {
	p := Plan("create REST API")

	for _, kw := range []string{
		"Subtask",
		"2-5",
		"Maximum 5 subtasks",
		"exactly ONE code or file operation",
	} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestCommitMessage(t *testing.T) {
	p := CommitMessage("+ added file", "M main.go", "add file")
	for _, kw := range []string{"Conventional Commits", "+ added file"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestCompareApproaches(t *testing.T) {
	p := CompareApproaches("create web server", "")
	for _, kw := range []string{"FUNDAMENTALLY DIFFERENT", "recommended_id"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestCodeModifyDiffForModel_Policies(t *testing.T) {
	for _, policy := range []string{"strict", "balanced", "advanced"} {
		t.Run(policy, func(t *testing.T) {
			p := CodeModifyDiffForModel("change", "code", policy)
			for _, kw := range []string{"SEARCH", "REPLACE"} {
				if !strings.Contains(p, kw) {
					t.Errorf("missing %q", kw)
				}
			}
		})
	}
}

func TestIntent(t *testing.T) {
	p := Intent("", "create a file", "")
	for _, kw := range []string{"intent router", `"mode"`} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestArticleSimple(t *testing.T) {
	p := ArticleSimple("Go concurrency", "", "English", "technical")
	for _, kw := range []string{"technical writer", "Go concurrency"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestArticlePlan(t *testing.T) {
	p := ArticlePlan("Go testing", "", "English", "technical", 5)
	for _, kw := range []string{"JSON", "Maximum 5 sections"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestPlanFull(t *testing.T) {
	p := PlanFull("create REST API", "")
	for _, kw := range []string{"goal", "subtasks", "Maximum 7 subtasks"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestReviewChanges(t *testing.T) {
	p := ReviewChanges("create API", "implement endpoint", "created main.go", "")
	for _, kw := range []string{"reviewer", "approved"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestVerifyCompletion(t *testing.T) {
	p := VerifyCompletion("create API", "created main.go", "")
	for _, kw := range []string{"verification", "completed"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestPRDescription(t *testing.T) {
	p := PRDescription("+ added code", "feature-branch")
	for _, kw := range []string{"Pull Request", "feature-branch"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestAgentInterviewQuestions(t *testing.T) {
	p := AgentInterviewQuestions(
		"add caching",
		"",
	)

	for _, kw := range []string{
		"clarifying questions",
		"3-5",
	} {
		if !strings.Contains(p, kw) {
			t.Errorf(
				"missing %q",
				kw,
			)
		}
	}
}

func TestAgentReflection(t *testing.T) {
	p := AgentReflection(
		"goal",
		"log",
		"prd",
		"gates",
		"diff",
	)

	for _, kw := range []string{
		"retrospective",
		"What Went Well",
	} {
		if !strings.Contains(
			p,
			kw,
		) {
			t.Errorf(
				"missing %q",
				kw,
			)
		}
	}
}

func TestExecutionStrategy(t *testing.T) {
	p := ExecutionStrategy(
		"create API",
		"",
		"medium",
		5,
	)

	for _, kw := range []string{
		"execution_mode",
		"simple|agent",
		"agent_depth",
		"normal|deep",
        "edit_mode",
        "patch|full",
	} {
		if !strings.Contains(p, kw) {
			t.Errorf(
				"missing %q",
				kw,
			)
		}
	}
}

func TestExecutionStrategyPromptSeparatesExecutionAndEditModes(
	t *testing.T,
) {
	p := ExecutionStrategy(
		"refactor the server into separate packages",
		"",
		"medium",
		6,
	)

	for _, want := range []string{
		"execution_mode",
		"edit_mode",
		"execution_mode and edit_mode are independent decisions",
		"PATCH is the DEFAULT",
		"FULL is NOT the default",
		"choose PATCH",
	} {
		if !strings.Contains(p, want) {
			t.Errorf(
				"missing %q",
				want,
			)
		}
	}
}

func TestSuggest(t *testing.T) {
	p := Suggest("package main", "")
	for _, kw := range []string{"Critical", "Tech Debt"} {
		if !strings.Contains(p, kw) {
			t.Errorf("missing %q", kw)
		}
	}
}

func TestAnalyzeDecisions(t *testing.T) {
	if p := AnalyzeDecisions("decision 1", ""); !strings.Contains(p, "decision debt") {
		t.Error("missing decision debt")
	}
}

func TestApproachSelection(t *testing.T) {
	approaches := []domain.Approach{{ID: 1, Name: "Simple", Description: "desc"}}
	p := ApproachSelection(approaches, "rec", "yes")
	if !strings.Contains(p, "select|new_task") {
		t.Error("missing action options")
	}
}
