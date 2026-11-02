package compatibility

import "path/filepath"

type Options struct {
	Directory    string
	Manifest     string
	FrontendSDK  string
	FrontendOnly bool
}

func CheckProject(options Options) ([]Record, error) {
	records := []Record{}
	if !options.FrontendOnly {
		record, err := ResolveGo(options.Directory)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	if options.FrontendSDK != "" {
		record, err := ResolveFrontend(options.Directory, options.FrontendSDK)
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	manifest := options.Manifest
	if !filepath.IsAbs(manifest) {
		manifest = filepath.Join(options.Directory, manifest)
	}
	data, err := readBounded(manifest, metadataLimit)
	if err != nil {
		return nil, err
	}
	return records, CheckManifest(data, records...)
}
