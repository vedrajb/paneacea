package terminal

import "fmt"

const (
	minColumns uint16 = 2
	minRows    uint16 = 1
)

func ValidateSize(columns, rows uint16) error {
	if columns > 1000 || rows > 500 {
		return fmt.Errorf("terminal dimensions exceed 1000 columns or 500 rows")
	}
	if columns < minColumns {
		return fmt.Errorf("columns must be at least %d", minColumns)
	}
	if rows < minRows {
		return fmt.Errorf("rows must be at least %d", minRows)
	}
	return nil
}
