package concurrency

import (
	"testing"
	"time"

	"quickpress/config"
)

func TestResolveScheduleUsesPerStageDuration(t *testing.T) {
	conc := config.Concurrency{
		Loop: -1,
		Unit: config.UnitSecond,
		Stages: []config.Stage{
			{Label: "预热", Duration: 10, Target: 2},
			{Label: "稳定", Duration: 20, Target: 6},
			{Label: "冲刺", Duration: 30, Target: 12},
		},
	}

	cases := []struct {
		name       string
		elapsed    time.Duration
		wantTarget int
		wantStage  int
		wantLoop   int
	}{
		{name: "first stage starts immediately", elapsed: 0, wantTarget: 2, wantStage: 0, wantLoop: 1},
		{name: "second stage starts after first duration", elapsed: 10 * time.Second, wantTarget: 6, wantStage: 1, wantLoop: 1},
		{name: "third stage starts after first two durations", elapsed: 30 * time.Second, wantTarget: 12, wantStage: 2, wantLoop: 1},
		{name: "next loop restarts after sum of durations", elapsed: 60 * time.Second, wantTarget: 2, wantStage: 0, wantLoop: 2},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			gotTarget, gotStage, gotLoop, finished := resolveSchedule(conc, tt.elapsed)
			if finished {
				t.Fatalf("resolveSchedule() finished unexpectedly")
			}
			if gotTarget != tt.wantTarget || gotStage != tt.wantStage || gotLoop != tt.wantLoop {
				t.Fatalf("resolveSchedule() = target %d stage %d loop %d, want target %d stage %d loop %d", gotTarget, gotStage, gotLoop, tt.wantTarget, tt.wantStage, tt.wantLoop)
			}
		})
	}
}

func TestResolveScheduleFinishesAfterLoopDurationSum(t *testing.T) {
	conc := config.Concurrency{
		Loop: 1,
		Unit: config.UnitSecond,
		Stages: []config.Stage{
			{Label: "预热", Duration: 10, Target: 2},
			{Label: "稳定", Duration: 20, Target: 6},
		},
	}

	_, _, _, finished := resolveSchedule(conc, 29*time.Second)
	if finished {
		t.Fatalf("resolveSchedule() finished before summed stage durations elapsed")
	}

	target, stage, loop, finished := resolveSchedule(conc, 30*time.Second)
	if !finished {
		t.Fatalf("resolveSchedule() did not finish after summed stage durations elapsed")
	}
	if target != 0 || stage != 1 || loop != 1 {
		t.Fatalf("resolveSchedule() finish state = target %d stage %d loop %d, want target 0 stage 1 loop 1", target, stage, loop)
	}
}

func TestSplitStagesByElapsedDuration(t *testing.T) {
	stages := []config.Stage{
		{Label: "预热", Duration: 10, Target: 2},
		{Label: "稳定", Duration: 20, Target: 6},
		{Label: "冲刺", Duration: 30, Target: 12},
	}

	prefix := stagesUpToDuration(stages, 15)
	if len(prefix) != 2 || prefix[0].Duration != 10 || prefix[1].Duration != 5 || prefix[1].Target != 6 {
		t.Fatalf("stagesUpToDuration() = %#v, want first stage plus 5 seconds of second stage", prefix)
	}

	suffix := stagesAfterDuration(stages, 15)
	if len(suffix) != 2 || suffix[0].Duration != 15 || suffix[0].Target != 6 || suffix[1].Duration != 30 {
		t.Fatalf("stagesAfterDuration() = %#v, want remaining 15 seconds of second stage plus third stage", suffix)
	}
}

func TestCompletedStageCutoffDurationSnapsToStageBoundary(t *testing.T) {
	conc := config.Concurrency{
		Loop: 1,
		Unit: config.UnitSecond,
		Stages: []config.Stage{
			{Label: "预热", Duration: 10, Target: 2},
			{Label: "稳定", Duration: 20, Target: 6},
			{Label: "冲刺", Duration: 30, Target: 12},
		},
	}

	if got := completedStageCutoffDuration(conc, 15*time.Second); got != 10 {
		t.Fatalf("completedStageCutoffDuration() = %d, want 10", got)
	}
	if got := completedStageCutoffDuration(conc, 30*time.Second); got != 30 {
		t.Fatalf("completedStageCutoffDuration() = %d, want 30", got)
	}
}
