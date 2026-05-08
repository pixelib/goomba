//go:build darwin

package embeddedsk

import "embed"

//go:embed apple-sdk-lite/*
//go:embed sdk_marker.txt
var Data embed.FS
