package model

import "testing"

func TestSummarize(t *testing.T) {
	got := Summarize([]Proxy{
		{Enabled: true, Status: "live"},
		{Enabled: true, Status: "failed"},
		{Enabled: false, Status: "live"},
		{Enabled: true, Status: "checking"},
	})
	if got.Total != 4 || got.Live != 1 || got.Failed != 1 || got.Disabled != 1 || got.Checking != 1 {
		t.Fatalf("unexpected summary: %+v", got)
	}
}
