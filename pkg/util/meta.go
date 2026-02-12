package util

import (
	"errors"
	"fmt"
)

const (
	APIVersionV1Alpha1 = "mcpchecker/v1alpha1"
	APIVersionV1Alpha2 = "mcpchecker/v1alpha2"
)

// ErrWrongKind indicates the YAML file has a different kind than expected
var ErrWrongKind = errors.New("wrong kind")

type TypeMeta struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind"`
}

func (t *TypeMeta) GetAPIVersion() string {
	if t.APIVersion == "" {
		return APIVersionV1Alpha1
	}

	return t.APIVersion
}

func (t *TypeMeta) Validate(expectedKind string) error {
	var err error
	err = errors.Join(err, ValidateAPIVersion(t.APIVersion))
	if t.Kind != expectedKind {
		err = errors.Join(err, fmt.Errorf("%w: got '%s', expected '%s'", ErrWrongKind, t.Kind, expectedKind))
	}

	return err
}

func ValidateAPIVersion(version string) error {
	switch version {
	case "", APIVersionV1Alpha1, APIVersionV1Alpha2:
		return nil
	default:
		return fmt.Errorf("unknown apiVersion: '%s", version)
	}
}
