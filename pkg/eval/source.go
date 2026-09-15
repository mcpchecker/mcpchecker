package eval

// SourceSpec defines an external task repository that can be fetched and used as task inputs.
type SourceSpec struct {
	// Repo is the repository URL, e.g. "github.com/org/repo"
	Repo string `json:"repo"`

	// Ref is the branch, tag, or commit to use. Defaults to HEAD when empty.
	Ref string `json:"ref,omitempty"`

	// Path is an optional subdirectory within the repository containing tasks.
	Path string `json:"path,omitempty"`

	// ServerMapping maps server names used in source tasks to server names in the
	// consumer's MCP config. e.g. {"kubernetes": "k8s-prod"}
	ServerMapping map[string]string `json:"serverMapping,omitempty"`
}
