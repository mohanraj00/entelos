// Package example supplies the ordinary-code baseline for a developer-owned rule.
package example

func Visible(state string, archived bool, allowedState string) bool {
	return state == allowedState && !archived
}
