package cli

import (
	"encoding/json"
	"fmt"
	"os"
)

// outputJSON marshals v as indented JSON to stdout.
func outputJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	_, err = fmt.Fprintln(os.Stdout, string(data))
	return err
}
