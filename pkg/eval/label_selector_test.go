package eval

import (
	"testing"
)

func TestParseLabelSelector(t *testing.T) {
	tests := []struct {
		name    string
		selector string
		want    map[string][]string
		wantErr bool
	}{
		{"empty string", "", nil, false},
		{"single label", "suite=kubernetes", map[string][]string{"suite": {"kubernetes"}}, false},
		{"different keys (AND)", "suite=kubernetes,difficulty=easy", map[string][]string{"suite": {"kubernetes"}, "difficulty": {"easy"}}, false},
		{"same key multiple values (OR)", "suite=kubernetes,suite=helm", map[string][]string{"suite": {"kubernetes", "helm"}}, false},
		{"mixed AND and OR", "suite=kubernetes,suite=helm,difficulty=easy", map[string][]string{"suite": {"kubernetes", "helm"}, "difficulty": {"easy"}}, false},
		{"whitespace around pairs", " suite=kubernetes , suite=helm ", map[string][]string{"suite": {"kubernetes", "helm"}}, false},
		{"duplicate same value deduped", "suite=kubernetes,suite=kubernetes", map[string][]string{"suite": {"kubernetes"}}, false},
		{"missing value", "suite", nil, true},
		{"empty key", "=value", nil, true},
		{"empty value", "suite=", nil, true},
		{"value with equals", "key=val=ue", map[string][]string{"key": {"val=ue"}}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLabelSelector(tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLabelSelector(%q) error = %v, wantErr %v", tt.selector, err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("ParseLabelSelector(%q) = %v, want %v", tt.selector, got, tt.want)
					return
				}
				for k, wantVals := range tt.want {
					gotVals := got[k]
					if len(gotVals) != len(wantVals) {
						t.Errorf("ParseLabelSelector(%q)[%q] = %v, want %v", tt.selector, k, gotVals, wantVals)
						continue
					}
					for i, v := range wantVals {
						if gotVals[i] != v {
							t.Errorf("ParseLabelSelector(%q)[%q][%d] = %q, want %q", tt.selector, k, i, gotVals[i], v)
						}
					}
				}
			}
		})
	}
}

func TestApplyLabelSelectorFilter(t *testing.T) {
	makeSpec := func(taskSets ...TaskSet) *EvalSpec {
		return &EvalSpec{
			Config: EvalConfig{
				TaskSets: taskSets,
			},
		}
	}

	tests := []struct {
		name         string
		spec         *EvalSpec
		selector     string
		wantTaskSets int
		wantErr      bool
	}{
		{
			name:    "nil spec",
			spec:    nil,
			selector: "suite=k8s",
			wantErr: true,
		},
		{
			name:         "empty selector",
			spec:         makeSpec(TaskSet{}),
			selector:     "",
			wantTaskSets: 1,
		},
		{
			name: "single label matches one",
			spec: makeSpec(
				TaskSet{LabelSelector: map[string]string{"suite": "k8s"}},
				TaskSet{LabelSelector: map[string]string{"suite": "helm"}},
			),
			selector:     "suite=k8s",
			wantTaskSets: 1,
		},
		{
			name: "OR on same key matches multiple taskSets",
			spec: makeSpec(
				TaskSet{LabelSelector: map[string]string{"suite": "k8s"}},
				TaskSet{LabelSelector: map[string]string{"suite": "helm"}},
				TaskSet{LabelSelector: map[string]string{"suite": "istio"}},
			),
			selector:     "suite=k8s,suite=helm",
			wantTaskSets: 2,
		},
		{
			name: "AND across keys narrows results",
			spec: makeSpec(
				TaskSet{LabelSelector: map[string]string{"suite": "k8s", "difficulty": "easy"}},
				TaskSet{LabelSelector: map[string]string{"suite": "k8s", "difficulty": "hard"}},
				TaskSet{LabelSelector: map[string]string{"suite": "helm"}},
			),
			selector:     "suite=k8s,difficulty=easy",
			wantTaskSets: 1,
		},
		{
			name: "OR and AND combined",
			spec: makeSpec(
				TaskSet{LabelSelector: map[string]string{"suite": "k8s", "difficulty": "easy"}},
				TaskSet{LabelSelector: map[string]string{"suite": "helm", "difficulty": "easy"}},
				TaskSet{LabelSelector: map[string]string{"suite": "istio", "difficulty": "easy"}},
			),
			selector:     "suite=k8s,suite=helm,difficulty=easy",
			wantTaskSets: 2,
		},
		{
			name: "no matches",
			spec: makeSpec(
				TaskSet{LabelSelector: map[string]string{"suite": "helm"}},
			),
			selector: "suite=k8s",
			wantErr:  true,
		},
		{
			name: "taskSet without labels expands for single value",
			spec: makeSpec(
				TaskSet{},
			),
			selector:     "suite=k8s",
			wantTaskSets: 1,
		},
		{
			name: "taskSet without labels expands for OR values",
			spec: makeSpec(
				TaskSet{},
			),
			selector:     "suite=k8s,suite=helm",
			wantTaskSets: 2,
		},
		{
			name:     "invalid selector format",
			spec:     makeSpec(TaskSet{}),
			selector: "invalid",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ApplyLabelSelectorFilter(tt.spec, tt.selector)
			if (err != nil) != tt.wantErr {
				t.Errorf("ApplyLabelSelectorFilter() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && tt.spec != nil {
				if len(tt.spec.Config.TaskSets) != tt.wantTaskSets {
					t.Errorf("got %d taskSets, want %d", len(tt.spec.Config.TaskSets), tt.wantTaskSets)
				}
			}
		})
	}
}
