package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/genmcp/gen-mcp/pkg/template"
)

type HttpStepConfig struct {
	URL     string            `json:"url"`
	Method  string            `json:"method"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    *HttpBody         `json:"body,omitempty"`
	Expect  *HttpExpect       `json:"expect,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
}

type HttpBody struct {
	Raw  *string        `json:"raw,omitempty"`
	JSON map[string]any `json:"json,omitempty"` // TODO: find a way to handle possibly templated values in the body
}

type HttpExpect struct {
	Status int         `json:"status,omitempty"`
	Body   *ExpectBody `json:"body,omitempty"`
}

type ExpectBody struct {
	Fields []FieldAssertion `json:"fields,omitempty"`
	Match  *string          `json:"match,omitempty"` // regex on raw body
}

type FieldAssertion struct {
	Path   string  `json:"path"`             // dot notation: "user.name", "items.0.id"
	Equals any     `json:"equals,omitempty"` // exact match
	Type   string  `json:"type,omitempty"`   // "string", "number", "array", "object", "bool", "null"
	Match  *string `json:"match,omitempty"`  // regex for string values
	Exists *bool   `json:"exists,omitempty"` // field presence check
}

type HttpStep struct {
	URL     *template.TemplateBuilder
	Method  *template.TemplateBuilder
	Headers map[string]*template.TemplateBuilder
	Body    *HttpBody
	Expect  *HttpExpect
	Timeout time.Duration
}

var _ StepRunner = &HttpStep{}

func ParseHttpStep(raw json.RawMessage) (StepRunner, error) {
	cfg := &HttpStepConfig{}

	err := json.Unmarshal(raw, cfg)
	if err != nil {
		return nil, err
	}

	return NewHttpStep(cfg)
}

func NewHttpStep(cfg *HttpStepConfig) (*HttpStep, error) {
	var err error
	step := &HttpStep{}

	sources := map[string]template.SourceFactory{
		"random": template.NewSourceFactory("random"),
	}
	parseOpts := template.TemplateParserOptions{Sources: sources}

	url, err := template.ParseTemplate(cfg.URL, parseOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse url: %w", err)
	}

	step.URL, err = template.NewTemplateBuilder(url, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create builder for url: %w", err)
	}

	method, err := template.ParseTemplate(cfg.Method, parseOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to parse method: %w", err)
	}

	step.Method, err = template.NewTemplateBuilder(method, false)
	if err != nil {
		return nil, fmt.Errorf("failed to create builder for method: %w", err)
	}

	step.Headers = make(map[string]*template.TemplateBuilder, len(cfg.Headers))
	for k, v := range cfg.Headers {
		h, err := template.ParseTemplate(v, parseOpts)
		if err != nil {
			return nil, fmt.Errorf("failed to parse header: %w", err)
		}

		step.Headers[k], err = template.NewTemplateBuilder(h, false)
		if err != nil {
			return nil, fmt.Errorf("failed to create builder for header: %w", err)
		}
	}

	step.Body = cfg.Body
	if err := step.Body.Validate(); err != nil {
		return nil, fmt.Errorf("invalid body for http step: %w", err)
	}

	step.Expect = cfg.Expect

	if cfg.Timeout != "" {
		timeout, err := time.ParseDuration(cfg.Timeout)
		if err != nil {
			return nil, fmt.Errorf("failed to parse timeout: %w", err)
		}
		step.Timeout = timeout
	} else {
		step.Timeout = DefaultTimeout
	}

	return step, nil
}

func (s *HttpStep) Execute(ctx context.Context, input *StepInput) (*StepOutput, error) {
	if input.Random != nil {
		s.URL.SetSourceResolver("random", input.Random)
		s.Method.SetSourceResolver("random", input.Random)
		for _, h := range s.Headers {
			h.SetSourceResolver("random", input.Random)
		}
	}

	for k, v := range input.Env {
		err := os.Setenv(k, v)
		if err != nil {
			return nil, fmt.Errorf("failed to set env var '%s' to value '%s': %w", k, v, err)
		}
	}
	defer func() {
		for k := range input.Env {
			_ = os.Unsetenv(k)
		}
	}()

	method, err := s.Method.GetResult()
	if err != nil {
		return nil, fmt.Errorf("failed to build method from template: %w", err)
	}

	url, err := s.URL.GetResult()
	if err != nil {
		return nil, fmt.Errorf("failed to build url from template: %w", err)
	}

	body, err := s.Body.Content()
	if err != nil {
		return nil, fmt.Errorf("failed to create reader for request body: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, s.Timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, method.(string), url.(string), body.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to create http request: %w", err)
	}

	// Apply configured headers
	for k, v := range s.Headers {
		headerVal, err := v.GetResult()
		if err != nil {
			return nil, fmt.Errorf("failed to build header %q from template: %w", k, err)
		}
		req.Header.Set(k, headerVal.(string))
	}

	// Set Content-Type from body if not explicitly configured
	if body.ContentType != "" && req.Header.Get("Content-Type") == "" {
		req.Header.Set("Content-Type", body.ContentType)
	}

	client := http.DefaultClient

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make http request: %w", err)
	}
	defer resp.Body.Close()

	return s.Expect.ValidateResponse(resp), nil
}

// BodyContent holds the serialized body and its content type.
type BodyContent struct {
	Reader      io.Reader
	ContentType string // empty if no content type should be set
}

func (b *HttpBody) Content() (*BodyContent, error) {
	if b == nil {
		return &BodyContent{Reader: bytes.NewReader(nil)}, nil
	}

	if b.Raw != nil {
		return &BodyContent{Reader: strings.NewReader(*b.Raw)}, nil
	}
	if b.JSON != nil {
		data, err := json.Marshal(b.JSON)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal body.json to json: %s", err)
		}

		return &BodyContent{
			Reader:      bytes.NewReader(data),
			ContentType: "application/json",
		}, nil
	}

	return nil, fmt.Errorf("no valid body set")
}

func (b *HttpBody) Validate() error {
	if b == nil {
		return nil
	}

	numDefined := 0
	if b.Raw != nil {
		numDefined++
	}
	if b.JSON != nil {
		numDefined++
	}

	if numDefined != 1 {
		return fmt.Errorf("exactly one key must be defined on body")
	}

	return nil
}

func (e *HttpExpect) ValidateResponse(resp *http.Response) *StepOutput {
	if e == nil {
		return &StepOutput{
			Type:    "http",
			Success: true,
			Message: "request completed (no expectations defined)",
		}
	}

	var errors []string

	// Validate status code
	if e.Status != 0 && e.Status != resp.StatusCode {
		errors = append(errors, fmt.Sprintf("expected status code %d, got %d", e.Status, resp.StatusCode))
	}

	// Validate body if configured
	if e.Body != nil {
		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			errors = append(errors, fmt.Sprintf("failed to read response body: %s", err))
		} else {
			errors = append(errors, e.Body.Validate(bodyBytes)...)
		}
	}

	out := &StepOutput{
		Type:    "http",
		Success: len(errors) == 0,
	}

	if out.Success {
		out.Message = "response passed all validation"
	} else {
		out.Error = fmt.Sprintf("response failed validation check: %s", strings.Join(errors, "; "))
	}

	return out
}

func (b *ExpectBody) Validate(body []byte) []string {
	if b == nil {
		return nil
	}

	var errors []string

	errors = append(errors, b.validateMatch(body)...)
	errors = append(errors, b.validateFields(body)...)

	return errors
}

func (b *ExpectBody) validateMatch(body []byte) []string {
	if b.Match == nil {
		return nil
	}

	re, err := regexp.Compile(*b.Match)
	if err != nil {
		return []string{fmt.Sprintf("invalid match regex %q: %s", *b.Match, err)}
	}

	if !re.Match(body) {
		return []string{fmt.Sprintf("body did not match pattern %q", *b.Match)}
	}

	return nil
}

func (b *ExpectBody) validateFields(body []byte) []string {
	if len(b.Fields) == 0 {
		return nil
	}

	// Empty body with field assertions is an error
	if len(body) == 0 {
		return []string{"expected JSON body but got empty response"}
	}

	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return []string{fmt.Sprintf("failed to parse response body as JSON: %s", err)}
	}

	var errors []string
	for _, field := range b.Fields {
		errors = append(errors, field.Validate(parsed)...)
	}

	return errors
}

func (f *FieldAssertion) Validate(data any) []string {
	value, exists := getFieldByPath(data, f.Path)

	// Check existence
	if f.Exists != nil {
		if *f.Exists && !exists {
			return []string{fmt.Sprintf("field %q does not exist", f.Path)}
		}
		if !*f.Exists && exists {
			return []string{fmt.Sprintf("field %q exists but should not", f.Path)}
		}
	}

	// If field doesn't exist and we're not checking existence, skip other validations
	if !exists {
		if f.Equals != nil || f.Type != "" || f.Match != nil {
			return []string{fmt.Sprintf("field %q does not exist", f.Path)}
		}
		return nil
	}

	var errors []string

	// Check type
	if f.Type != "" {
		if err := validateType(value, f.Type, f.Path); err != nil {
			errors = append(errors, err.Error())
		}
	}

	// Check equals
	if f.Equals != nil {
		if !valuesEqual(value, f.Equals) {
			errors = append(errors, fmt.Sprintf("field %q: expected %v, got %v", f.Path, f.Equals, value))
		}
	}

	// Check match (regex)
	if f.Match != nil {
		str, ok := value.(string)
		if !ok {
			errors = append(errors, fmt.Sprintf("field %q: match requires string value, got %T", f.Path, value))
		} else {
			re, err := regexp.Compile(*f.Match)
			if err != nil {
				errors = append(errors, fmt.Sprintf("field %q: invalid match regex %q: %s", f.Path, *f.Match, err))
			} else if !re.MatchString(str) {
				errors = append(errors, fmt.Sprintf("field %q: value %q did not match pattern %q", f.Path, str, *f.Match))
			}
		}
	}

	return errors
}

type pathPart struct {
	key     string
	isIndex bool
}

func getFieldByPath(data any, path string) (any, bool) {
	parts := splitPath(path)
	current := data

	for _, part := range parts {
		if part.isIndex {
			arr, ok := current.([]any)
			if !ok {
				return nil, false
			}
			var idx int
			if _, err := fmt.Sscanf(part.key, "%d", &idx); err != nil {
				return nil, false
			}
			if idx < 0 || idx >= len(arr) {
				return nil, false
			}
			current = arr[idx]
		} else {
			obj, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			val, ok := obj[part.key]
			if !ok {
				return nil, false
			}
			current = val
		}
	}

	return current, true
}

// splitPath splits a path like "items[0].name" or "data.users[2].email"
// into parts with type information to distinguish array indices from object keys.
// Examples:
//
//	"items[0].name"      -> [{key: "items", isIndex: false}, {key: "0", isIndex: true}, {key: "name", isIndex: false}]
//	"data.0.field"       -> [{key: "data", isIndex: false}, {key: "0", isIndex: false}, {key: "field", isIndex: false}]
func splitPath(path string) []pathPart {
	var parts []pathPart
	var current strings.Builder
	inBracket := false

	for i := 0; i < len(path); i++ {
		ch := path[i]
		switch ch {
		case '.':
			if current.Len() > 0 {
				parts = append(parts, pathPart{key: current.String(), isIndex: inBracket})
				current.Reset()
			}
			inBracket = false
		case '[':
			if current.Len() > 0 {
				parts = append(parts, pathPart{key: current.String(), isIndex: false})
				current.Reset()
			}
			inBracket = true
		case ']':
			if current.Len() > 0 {
				parts = append(parts, pathPart{key: current.String(), isIndex: true})
				current.Reset()
			}
			inBracket = false
		default:
			current.WriteByte(ch)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, pathPart{key: current.String(), isIndex: inBracket})
	}

	return parts
}

func validateType(value any, expectedType string, path string) error {
	var actualType string

	switch value.(type) {
	case string:
		actualType = "string"
	case float64:
		actualType = "number"
	case bool:
		actualType = "bool"
	case []any:
		actualType = "array"
	case map[string]any:
		actualType = "object"
	case nil:
		actualType = "null"
	default:
		actualType = fmt.Sprintf("%T", value)
	}

	if actualType != expectedType {
		return fmt.Errorf("field %q: expected type %s, got %s", path, expectedType, actualType)
	}

	return nil
}

func valuesEqual(a, b any) bool {
	// JSON unmarshals numbers as float64, YAML may use int.
	// Handle the common cross-format comparison case.
	if aFloat, ok := a.(float64); ok {
		if bInt, ok := b.(int); ok {
			return aFloat == float64(bInt)
		}
	}
	if aInt, ok := a.(int); ok {
		if bFloat, ok := b.(float64); ok {
			return float64(aInt) == bFloat
		}
	}

	return fmt.Sprintf("%v", a) == fmt.Sprintf("%v", b)
}
