//go:build !windows

package launcher

import (
	"os"
	"os/user"
	"strconv"
)

var (
	lookupNativeServiceOwner   = defaultLookupNativeServiceOwner
	chownNativeServiceMetadata = func(file *os.File, uid, gid int) error {
		return file.Chown(uid, gid)
	}
)

func defaultLookupNativeServiceOwner(username string) (int, int, error) {
	account, err := user.Lookup(username)
	if err != nil {
		return 0, 0, err
	}
	uid, err := strconv.Atoi(account.Uid)
	if err != nil {
		return 0, 0, err
	}
	gid, err := strconv.Atoi(account.Gid)
	if err != nil {
		return 0, 0, err
	}
	return uid, gid, nil
}
