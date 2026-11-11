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

func TestReplaceFileDropsAnACLInheritedFromTheDirectory(t *testing.T) {
	dir := t.TempDir()
	destination := filepath.Join(dir, "photo.jpg")
	source := filepath.Join(t.TempDir(), "rewritten.jpg")
	_ = os.WriteFile(destination, []byte("old"), 0644)
	_ = os.WriteFile(source, []byte("new"), 0644)
	if output, err := exec.Command("/bin/chmod", "+a", "everyone allow read,file_inherit", dir).CombinedOutput(); err != nil {
		t.Skipf("cannot set an inheritable ACL here: %v %s", err, output)
	}
	t.Cleanup(func() { _ = exec.Command("/bin/chmod", "-RN", dir).Run() })
	original, _ := os.Lstat(destination)
	if hasACL(destination) {
		t.Fatal("the original predates the directory ACL and must have none")
	}
	if err := replaceFile(destination, source, original); err != nil {
		t.Fatalf("replace: %v", err)
	}
	if hasACL(destination) {
		t.Fatal("the rewritten file must not gain the directory's inheritable ACL")
	}
}
