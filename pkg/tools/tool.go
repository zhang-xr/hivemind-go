package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
)

type Tool interface {
	Name() string

	Description() string

	Parameters() json.RawMessage

	Execute(ctx context.Context, args map[string]interface{}) (string, error)
}

func FormatForPrompt(t Tool) (string, error) {
	params := t.Parameters()

	var indentedParams bytes.Buffer

	if err := json.Indent(&indentedParams, params, "", "  "); err != nil {

		return "", fmt.Errorf("failed to indent parameters JSON for tool %s: %w", t.Name(), err)
	}

	return fmt.Sprintf(
		"Tool Name: %s\nDescription: %s\nParameters Schema: %s",
		t.Name(),
		t.Description(),
		indentedParams.String(),
	), nil
}
