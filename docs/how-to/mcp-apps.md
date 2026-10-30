# Evaluate MCP Apps resources

Apps assertions check that an invoked MCP tool provides a retrievable HTML UI
resource. This initial implementation does not execute JavaScript, render the UI,
or check tables, elements, or displayed data. No browser or JavaScript runtime is
required.

Apps assertions activate resource validation automatically. They work with any
agent that uses mcpchecker’s recording proxy; the agent itself does not need Apps
support. This evaluates the server's UI resource contract after an agent chooses
to call a tool; it does not evaluate the client's ability to display or use an
MCP App. This example uses the builtin agent and enables Apps protocol handling:

```yaml
kind: Eval
metadata:
  name: namespace-app
config:
  agent:
    type: builtin.llm-agent
    model: openai:gpt-5
    apps:
      host: true
  mcpConfigFile: ./mcp-config.yaml
  taskSets:
    - path: ./namespaces.yaml
```

`config.agent.apps.host` defaults to `false`. It controls how mcpchecker presents
itself to the server and what it forwards to the agent. With `host: true`, mcpchecker
provides a limited subset of Apps host behavior on behalf of the agent: it
advertises the UI capability to upstream servers, hides app-only tools from the
model, and removes `ui` and `ui/*` keys from tool result metadata sent to the agent.
Filtering covers the result itself, content blocks, and embedded resource
metadata, including nested tool-result content. Other metadata and payloads are
preserved, and the recording retains the original tool result. This setting also works without Apps
assertions and does not trigger resource reads by itself. It does not make the
client Apps-capable: mcpchecker does not start a browser, render the UI, or
implement app-to-host messaging. Use it to exercise the server's behavior when
UI support is declared, even if the agent client has no Apps support.

Like agent-specific skill settings, this belongs to the agent configuration rather
than the eval's assertions. For a reusable `kind: Agent` file, put `apps` at the
top level alongside `builtin`, `acp`, or `commands`:

```yaml
kind: Agent
metadata:
  name: namespace-agent
builtin:
  type: llm-agent
  model: openai:gpt-5
apps:
  host: true
```

Reference that file with `config.agent.type: file` and `config.agent.path`.
An explicit `config.agent.apps` overrides the file's Apps setting, including
`host: false`; omitting it preserves the file's setting.

For a client with native Apps support, leave host handling disabled so mcpchecker
preserves UI result metadata and app-only tools for that client. This is not full
Apps negotiation passthrough: the upstream connection is initialized by
mcpchecker before the agent connects, and the agent's UI capability declaration
is not forwarded. Servers that require that declaration may therefore withhold
UI resources. Native-client rendering and app-to-host messaging are outside this
resource validation feature.

Apps assertions work with either host setting. Adding an assertion does not
change capability negotiation or the model's tool surface. With `host: false`,
validation checks whatever UI the server provides without declaring UI support;
servers that require that declaration may only provide UI with `host: true`.

The agent model needs its usual provider credentials, such as `OPENAI_API_KEY`
for this example. No judge model is needed for the Apps assertion.

In `namespaces.yaml`:

```yaml
kind: Task
apiVersion: mcpchecker/v1alpha2
metadata:
  name: namespaces-ui
spec:
  prompt:
    inline: Call namespaces_list to list the namespaces.
  assertions:
    requirePresent: true
    apps:
      - server: kubernetes
        tool: namespaces_list
```

Use the server name from your MCP configuration and the exact tool name.
The same assertion works for any UI shape, including pod details: change the
tool name to the one that provides that UI. Assertions can also be configured
under eval task sets and compose with task-level assertions.

In this example, the agent decides to invoke `namespaces_list` and supplies its
arguments. The recording proxy then reads the referenced UI resource and checks
it automatically because of the Apps assertion. The agent does not choose to
open the UI or request that validation, and validation does not send the HTML to
the agent. A passing assertion means the tool was invoked and every matching
invocation provided a resource satisfying the checks below. It does not mean the
client displayed the app, that its JavaScript ran successfully, or that the UI
shows the namespaces correctly.

For every matching invocation, mcpchecker checks:

- The tool call returns a result without a transport/protocol error.
- The call result's top-level `_meta.ui.resourceUri` contains a nonempty string,
  falling back to the tool definition's metadata when no usable result reference
  is found. Legacy `_meta["ui/resourceUri"]` is accepted in both places. Per-call
  references take precedence and are fetched for that invocation. References
  inside content blocks are not used for resource discovery.
- The reference is a syntactically valid `ui://` URI identifying a resource.
  Invalid references fail with `invalid_ui_resource_uri` before any resource read.
- The resource can be read through the same MCP session, even when it is absent
  from `resources/list`.
- The response contains exactly one resource with the referenced URI and MIME
  type `text/html;profile=mcp-app`.
- Its text or decoded blob contains non-whitespace content of at most 1 MiB.

These checks establish that the resource is available and advertised as HTML;
they do not validate HTML syntax or prove that the app works when rendered.
Tool results with `isError: true` can still provide a UI, so task verification
should separately check tool success when required.

Apps uses the same capability filtering as other tool assertions, including the
configured tool allowlist. If the server is absent or the target is absent from
its allowed, model-visible tools, the assertion is skipped by default;
`requirePresent: true` makes it a presence failure instead. The example requires
presence so a missing tool fails the check. With `host: true`, app-only tools are
hidden and treated as absent.

When a tool is present but the agent never calls it, the assertion fails with
`tool_not_invoked`. An invoked tool with a missing UI reference, unreadable
resource, or invalid MIME type fails with a resource validation code such as
`missing_ui_resource`, `ui_resource_read_failed`, or `invalid_ui_mime_type`.
Read failures can also result from transport errors or cancellation. Every
matching invocation must pass, so a later success cannot hide an earlier failure.
Resource reads have a ten-second deadline and inherit task cancellation through
the proxy's HTTP request context. At task completion, unfinished validation calls
are canceled and their observations are recorded before taking the final snapshot.

The tool runs once. After the call, the recording proxy reads the UI resource
through its existing upstream MCP session; neither HTML nor the resource response
is sent to the agent by validation. The agent's `apps.host` separately controls tool
visibility and UI metadata filtering as described above.

`assertionResults.apps` contains the assertion verdict and failure details. When
all Apps targets are filtered out, this field is omitted; check
`skippedAssertions` or `presenceFailures` instead. `appObservations` contains one
entry per matching invocation, with server, tool, generated invocation ID, and
the resource URI and validated MIME type when available. It is omitted when
there are no observations. Failures include a code and, for tool-call or read
errors, a bounded diagnostic in `detail`: a single line of at most 512 characters,
with HTML response bodies omitted. Assertion output includes the URI and
diagnostic when available.

Checker UI reads are recorded in resource history with `hostUiRead: true` and
the observation's invocation ID in `appCallId`, even with the agent's
`apps.host: false`.
They retain the URI and any available MIME type, content byte size, and bounded
read-error diagnostic; the HTML result is removed. The CLI counts these as
`uiReads`. A successful resource read can still fail Apps validation, for example
because its MIME type is wrong. These reads do not count toward agent resource
assertions, call-order checks, or model token estimates. Ordinary resource
recording is unchanged.

## Run locally

The [example configuration](../../examples/mcp-apps/) includes the eval, task,
MCP connection, and server settings. For a sibling `kubernetes-mcp-server`
checkout, start the server from the mcpchecker repository root in a separate
terminal:

```sh
cd ../kubernetes-mcp-server
go run ./cmd/kubernetes-mcp-server --config ../mcpchecker/examples/mcp-apps/server.toml
```

This serves read-only core tools with Apps enabled at `http://localhost:8080/mcp`
and uses `KUBECONFIG` or `~/.kube/config` for cluster access. Restart the server
after changing its Apps setting. The server needs no model API key.

Adjust `mcp-config.yaml` for your server URL and `namespaces.yaml` for your tool
name. Set your model provider credentials, such as `OPENAI_API_KEY`, and run from
the mcpchecker repository root:

```sh
go run ./cmd/mcpchecker check examples/mcp-apps/eval.yaml
jq '.results[] | {assertions: .assertionResults.apps, observations: .appObservations}' mcpchecker-namespace-app-out.json
```
