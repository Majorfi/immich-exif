//go:build windows

package process

func hasACL(string) bool {
	return true
}
