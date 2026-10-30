package eval

import (
	"fmt"

	"github.com/mcpchecker/mcpchecker/pkg/mcpapps"
)

func evaluateApps(assertions []mcpapps.Assertion, observations []mcpapps.Observation) *SingleAssertionResult {
	if len(assertions) == 0 {
		return nil
	}
	res := &SingleAssertionResult{Passed: true}
	fail := func(message string) {
		res.Passed = false
		res.Details = append(res.Details, message)
		if res.Reason == "" {
			res.Reason = message
		}
	}
	for _, a := range assertions {
		found := false
		for _, o := range observations {
			// Assertion sets share the task's calls. Only this exact target's
			// observations apply, regardless of which set requested collection.
			if o.Server != a.Server || o.Tool != a.Tool {
				continue
			}
			found = true
			label := fmt.Sprintf("%s/%s %s", o.Server, o.Tool, o.CallID)
			if o.URI != "" {
				label += " (" + o.URI + ")"
			}
			if o.Code != "" {
				message := label + ": " + o.Code
				if o.Detail != "" {
					message += ": " + o.Detail
				}
				fail(message)
				continue
			}
			if o.URI == "" || o.MIMEType == "" {
				fail(label + ": missing_ui_observation")
				continue
			}
		}
		if !found {
			fail(fmt.Sprintf("%s/%s: tool_not_invoked", a.Server, a.Tool))
		}
	}
	return res
}
