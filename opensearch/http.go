package main

import (
	"net/http"

	"github.com/flanksource/incident-commander/plugin/sdk"
)

func (p *OpenSearchPlugin) HTTPHandler() http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/version", sdk.VersionHandler(sdk.BuildInfo{
		Name:       pluginName,
		Version:    Version,
		BuildDate:  BuildDate,
		UIChecksum: uiChecksum,
	}))
	return mux
}
