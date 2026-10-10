package testutil

import (
	"errors"
	"net/url"
	"strings"
)

func isolatedPostgresDSN(dsn, schema string) (string, error) {
	if strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://") {
		connectionURL, err := url.Parse(dsn)
		if err != nil {
			return "", errors.New("invalid PostgreSQL connection URL")
		}
		parameters, err := url.ParseQuery(connectionURL.RawQuery)
		if err != nil {
			return "", errors.New("invalid PostgreSQL connection URL")
		}
		parameters.Set("search_path", schema)
		connectionURL.RawQuery = parameters.Encode()
		return connectionURL.String(), nil
	}
	return dsn + " search_path=" + schema, nil
}
