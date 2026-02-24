package task

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolvePromptTemplates(t *testing.T) {
	tests := []struct {
		name         string
		prompt       string
		setupOutputs map[string]map[string]string
		want         string
	}{
		{
			name:         "no templates",
			prompt:       "Create a VM in the default namespace",
			setupOutputs: map[string]map[string]string{},
			want:         "Create a VM in the default namespace",
		},
		{
			name:   "single template resolved",
			prompt: "Create a VM in the {steps.k8s.createNamespace.namespace} namespace",
			setupOutputs: map[string]map[string]string{
				"k8s.createNamespace": {
					"namespace": "vm-test-abc123",
				},
			},
			want: "Create a VM in the vm-test-abc123 namespace",
		},
		{
			name:   "multiple templates resolved",
			prompt: "Create a VM in {steps.k8s.createNamespace.namespace} and check {steps.k8s.createNamespace.namespace} exists",
			setupOutputs: map[string]map[string]string{
				"k8s.createNamespace": {
					"namespace": "vm-test-abc123",
				},
			},
			want: "Create a VM in vm-test-abc123 and check vm-test-abc123 exists",
		},
		{
			name:   "missing output reference returns original",
			prompt: "Create a VM in the {steps.k8s.createNamespace.namespace} namespace",
			setupOutputs: map[string]map[string]string{
				"other.step": {
					"key": "value",
				},
			},
			want: "Create a VM in the {steps.k8s.createNamespace.namespace} namespace",
		},
		{
			name:         "no steps marker not processed",
			prompt:       "Create a VM in the {other.variable} namespace",
			setupOutputs: map[string]map[string]string{},
			want:         "Create a VM in the {other.variable} namespace",
		},
		{
			name:         "nil setupOutputs",
			prompt:       "Create a VM in the {steps.k8s.createNamespace.namespace} namespace",
			setupOutputs: nil,
			want:         "Create a VM in the {steps.k8s.createNamespace.namespace} namespace",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &taskRunner{
				prompt:       tt.prompt,
				setupOutputs: tt.setupOutputs,
			}

			got := r.resolvePromptTemplates(tt.prompt)
			assert.Equal(t, tt.want, got)
		})
	}
}
