//go:build linux

package herdr

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestSocketIdentityRejectsReusedInode(t *testing.T) {
	// Model unlink/rebind by the same server process on a filesystem that
	// immediately recycles the socket inode. The /tmp tmpfs used locally does
	// not reproduce this allocation pattern, so do not depend on the allocator.
	original := unix.Stat_t{Dev: 1, Ino: 23, Ctim: unix.Timespec{Sec: 100, Nsec: 17}}
	for _, change := range []string{"second", "nanosecond"} {
		t.Run(change, func(t *testing.T) {
			replacement := original
			if change == "second" {
				replacement.Ctim.Sec++
			} else {
				replacement.Ctim.Nsec++
			}
			if socketIdentity(original) == socketIdentity(replacement) {
				t.Fatal("replacement socket reused the original identity")
			}
		})
	}
	// Ordinary access must not invalidate a saved binding.
	accessed := original
	accessed.Atim.Sec++
	if socketIdentity(original) != socketIdentity(accessed) {
		t.Fatal("access time changed socket identity")
	}
}
