package app

import "testing"

func TestDefaultStages_FullPipelineIncludesDocumentedRolesAndPersistStage(t *testing.T) {
	stages := defaultStages("full-pipeline")
	if len(stages) != 9 {
		t.Fatalf("expected 9 stages including persist, got %d", len(stages))
	}
	expectedOrder := []string{"plan", "compose", "write", "observe", "reflect", "normalize", "audit", "revise", "persist"}
	for i, want := range expectedOrder {
		if stages[i].Name != want {
			t.Fatalf("stage order[%d] = %q, want %q", i, stages[i].Name, want)
		}
	}

	byName := make(map[string]struct {
		phase          string
		jobTitle       string
		responsibility string
	})
	for _, stage := range stages {
		byName[stage.Name] = struct {
			phase          string
			jobTitle       string
			responsibility string
		}{
			phase:          stage.Phase,
			jobTitle:       stage.JobTitle,
			responsibility: stage.Responsibility,
		}
	}

	cases := map[string]struct {
		phase          string
		jobTitle       string
		responsibility string
	}{
		"plan": {
			phase:          "planning",
			jobTitle:       "章节规划师",
			responsibility: "决定这一章要完成什么、推进哪些 hooks、遵守哪些硬约束",
		},
		"write": {
			phase:          "writing",
			jobTitle:       "章节写手",
			responsibility: "基于规划和上下文写出章节正文，并产出可结算的章节结果",
		},
		"audit": {
			phase:          "auditing",
			jobTitle:       "连续性审计师",
			responsibility: "检查设定、时间线、角色、节奏和文本风险",
		},
		"revise": {
			phase:          "revising",
			jobTitle:       "修稿编辑",
			responsibility: "按审计问题做定点修复或受控改写",
		},
		"persist": {
			phase:          "persisting",
			jobTitle:       "持久化执行器",
			responsibility: "把章节正文、truth files、快照和索引同步写入磁盘",
		},
	}

	for name, want := range cases {
		got, ok := byName[name]
		if !ok {
			t.Fatalf("expected stage %q to exist", name)
		}
		if got.phase != want.phase || got.jobTitle != want.jobTitle || got.responsibility != want.responsibility {
			t.Fatalf("stage %q mismatch: got %+v want %+v", name, got, want)
		}
	}
}
