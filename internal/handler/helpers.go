package handler

import (
	"fmt"

	"github.com/labstack/echo/v4"
)

// isHTMX returns true if the request was made by htmx.
func isHTMX(c echo.Context) bool {
	return c.Request().Header.Get("HX-Request") == "true"
}

// formatDuration converts milliseconds to a human-readable duration string.
func formatDuration(ms int) string {
	if ms < 1000 {
		return fmt.Sprintf("%dms", ms)
	}
	return fmt.Sprintf("%.1fs", float64(ms)/1000)
}
