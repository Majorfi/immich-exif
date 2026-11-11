//go:build darwin

package process

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestHasACLDetectsAFileACL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "photo.jpg")
	if err := os.WriteFile(path, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if hasACL(path) {
		t.Fatal("a plain file has no ACL")
	}
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read", path).CombinedOutput(); err != nil {
		t.Skipf("cannot set an ACL here: %v %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-N", path).Run() })
	if !hasACL(path) {
		t.Fatal("the ACL must be detected")
	}
}
