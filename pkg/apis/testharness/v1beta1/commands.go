package v1beta1

import (
	"fmt"
	"strings"

	"github.com/IGLOU-EU/go-wildcard"
)

func (c *CommandOutput) ValidateCommandOutput(stdoutOutput, stderrOutput strings.Builder) error {
	var errs []string

	if c.Stdout != nil {
		if err := c.Stdout.validateOutput("stdout", stdoutOutput.String()); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if c.Stderr != nil {
		if err := c.Stderr.validateOutput("stderr", stderrOutput.String()); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("validation errors: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (e *ExpectedOutput) validateOutput(outputType string, actualValue string) error {
	expectedValue := e.ExpectedValue
	matchType := e.MatchType
	if matchType == "" {
		matchType = MatchEquals
	}
	switch matchType {
	case MatchContains:
		if !strings.Contains(actualValue, expectedValue) {
			return fmt.Errorf("expected %s to contain: %s, but it did not", outputType, expectedValue)
		}

	case MatchWildcard:
		if !wildcard.Match(expectedValue, actualValue) {
			return fmt.Errorf("%s did not match wildcard pattern: %s", outputType, expectedValue)
		}

	default: // MatchEquals
		if actualValue != expectedValue {
			return fmt.Errorf("expected exact %s: %s, got: %s", outputType, expectedValue, actualValue)
		}
	}

	return nil
}
