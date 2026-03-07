package config

import (
	"encoding/json"
	"io"

	"github.com/typesanitizer/happygo/common/errorx"
)

// Load reads workspace configuration from JSON and validates it.
func Load(r io.Reader) (WorkspaceConfig, error) {
	var wcJSON WorkspaceConfigJSON
	if err := json.NewDecoder(r).Decode(&wcJSON); err != nil {
		return WorkspaceConfig{}, errorx.Wrapf("+stacks", err, "parse workspace configuration")
	}
	wc, err := wcJSON.Validate()
	if err != nil {
		return WorkspaceConfig{}, err
	}
	return wc, nil
}
