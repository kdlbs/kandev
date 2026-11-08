package mergestate

import (
	"strconv"
	"strings"
)

// ChangeKey identifies one pull request or merge request: provider, canonical
// host, repository ID and change number.
type ChangeKey struct {
	Provider string
	Host     string
	RepoID   string
	Number   int
}

func (k ChangeKey) host() string {
	return strings.TrimSuffix(strings.ToLower(strings.TrimSpace(k.Host)), ".")
}

// String is the stable canonical form used as a storage key.
func (k ChangeKey) String() string {
	return k.Provider + "|" + k.host() + "|" + k.RepoID + "|" + strconv.Itoa(k.Number)
}

// Less orders by provider, host, repository ID and numeric change number.
// Repository IDs that are all digits compare numerically.
func (k ChangeKey) Less(o ChangeKey) bool {
	if k.Provider != o.Provider {
		return k.Provider < o.Provider
	}
	if k.host() != o.host() {
		return k.host() < o.host()
	}
	if k.RepoID != o.RepoID {
		return repoIDLess(k.RepoID, o.RepoID)
	}
	return k.Number < o.Number
}

func repoIDLess(a, b string) bool {
	an, aerr := strconv.ParseInt(a, 10, 64)
	bn, berr := strconv.ParseInt(b, 10, 64)
	if aerr == nil && berr == nil {
		return an < bn
	}
	return a < b
}
