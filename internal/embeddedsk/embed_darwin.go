//go:build darwin

package embeddedsk

import "embed"

//go:embed sdk.zip
//go:embed sdk_marker.txt
var Data embed.FS
