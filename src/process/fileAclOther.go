//go:build !darwin

package process

func hasACL(string) bool {
	return false
}
