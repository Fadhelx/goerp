package testutil

import "testing"

func TempDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}
