package eval

import (
	"fmt"
	"strings"
)

// ParseLabelSelector parses a comma-separated label selector string into a multi-value map.
// Format: "key1=value1,key2=value2,key1=value3".
// Different keys use AND semantics; duplicate keys use OR semantics.
// Example: "suite=kubernetes,suite=helm,difficulty=easy" means
// (suite=kubernetes OR suite=helm) AND difficulty=easy.
func ParseLabelSelector(selector string) (map[string][]string, error) {
	selector = strings.TrimSpace(selector)
	if selector == "" {
		return nil, nil
	}

	labels := make(map[string][]string)
	for _, pair := range strings.Split(selector, ",") {
		parts := strings.SplitN(strings.TrimSpace(pair), "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid label selector format, expected key=value, got: %s", pair)
		}
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" || value == "" {
			return nil, fmt.Errorf("label selector key and value cannot be empty")
		}
		labels[key] = appendUnique(labels[key], value)
	}

	return labels, nil
}

func appendUnique(slice []string, val string) []string {
	for _, s := range slice {
		if s == val {
			return slice
		}
	}
	return append(slice, val)
}

// ApplyLabelSelectorFilter applies a CLI-provided label selector to an EvalSpec
// by merging it into each taskSet's LabelSelector.
// Different keys use AND semantics; duplicate keys use OR semantics.
// Example: "suite=kubernetes,suite=helm" keeps taskSets matching either suite.
//
// For taskSets that don't already have a key set, the taskSet is expanded into
// one copy per value so that tasks are filtered correctly at the task level.
//
// This is intentionally kept in the eval package so filtering logic is consolidated
// outside of the CLI layer.
func ApplyLabelSelectorFilter(spec *EvalSpec, selector string) error {
	if spec == nil {
		return fmt.Errorf("eval spec cannot be nil")
	}

	labels, err := ParseLabelSelector(selector)
	if err != nil {
		return err
	}
	if len(labels) == 0 {
		return nil
	}

	var filteredTaskSets []TaskSet
	for _, ts := range spec.Config.TaskSets {
		if ts.LabelSelector == nil {
			ts.LabelSelector = make(map[string]string)
		}

		// Check compatibility: if the taskSet already has a value for a key,
		// it must be one of the requested values.
		compatible := true
		for key, values := range labels {
			if existing, exists := ts.LabelSelector[key]; exists {
				if !containsValue(values, existing) {
					compatible = false
					break
				}
			}
		}
		if !compatible {
			continue
		}

		// Expand: for keys not already set on the taskSet, create a copy per value.
		expanded := expandTaskSet(ts, labels)
		filteredTaskSets = append(filteredTaskSets, expanded...)
	}

	if len(filteredTaskSets) == 0 {
		return fmt.Errorf("no taskSets match label selector: %s", selector)
	}

	spec.Config.TaskSets = filteredTaskSets

	return nil
}

// expandTaskSet creates copies of a taskSet for each combination of unset label values.
// Keys already present on the taskSet are left as-is.
func expandTaskSet(ts TaskSet, labels map[string][]string) []TaskSet {
	result := []TaskSet{ts}

	for key, values := range labels {
		if _, exists := ts.LabelSelector[key]; exists {
			// Already set — no expansion needed for this key
			continue
		}

		// Expand current result set: for each existing copy, create one per value
		var expanded []TaskSet
		for _, r := range result {
			for _, val := range values {
				copy := copyTaskSet(r)
				copy.LabelSelector[key] = val
				expanded = append(expanded, copy)
			}
		}
		result = expanded
	}

	return result
}

func copyTaskSet(ts TaskSet) TaskSet {
	newSelector := make(map[string]string, len(ts.LabelSelector))
	for k, v := range ts.LabelSelector {
		newSelector[k] = v
	}
	ts.LabelSelector = newSelector
	return ts
}

func containsValue(values []string, target string) bool {
	for _, v := range values {
		if v == target {
			return true
		}
	}
	return false
}

// matchesLabelSelector checks if the task labels match the label selector.
// All labels in the selector must match (AND logic).
// Returns true if selector is empty or nil.
func matchesLabelSelector(taskLabels, selector map[string]string) bool {
	if len(selector) == 0 {
		return true
	}

	for key, value := range selector {
		taskValue, exists := taskLabels[key]
		if !exists || taskValue != value {
			return false
		}
	}

	return true
}
