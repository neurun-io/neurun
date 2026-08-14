package browser

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"
)

// The values a coherent identity is assembled from: the operating systems and
// platform versions that ship together, the browser versions that exist, the
// screens and GPUs real machines report, and what a country implies about
// language and clock.
//
// It is reference data, not per-organization state, so it is embedded and
// served rather than stored. The point is that a caller picks from it instead of
// inventing values: an OS version nobody shipped, or a GPU string that does not
// match the platform claiming it, is the contradiction a detector looks for.
//
//go:embed catalog.json
var catalogJSON []byte

type Catalog struct {
	OperatingSystems []CatalogOS      `json:"operating_systems"`
	Devices          []CatalogDevice  `json:"devices"`
	Browsers         []CatalogBrowser `json:"browsers"`
	Screens          []CatalogScreen  `json:"screens"`
	// DensityPixelRatios pair with a screen to give the physical resolution.
	DensityPixelRatios  []float32    `json:"density_pixel_ratios"`
	HardwareConcurrency []uint32     `json:"hardware_concurrency"`
	Memory              []uint32     `json:"memory"`
	Geos                []CatalogGeo `json:"geos"`
}

// CatalogOS carries the fields an operating system fixes for everything under
// it: a Windows install reports Win32, x86 and 64, runs Chrome but never
// Safari, and has its own releases and its own cards.
type CatalogOS struct {
	OS OS `json:"os"`
	// FormFactor is "desktop" or "mobile". A mobile system carries no platform,
	// releases or cards of its own: the handset does, so pick a device first.
	FormFactor        string             `json:"form_factor"`
	NavigatorPlatform string             `json:"navigator_platform"`
	Bitness           string             `json:"bitness"`
	Architecture      string             `json:"architecture"`
	Browsers          []Browser          `json:"browsers"`
	Versions          []CatalogOSVersion `json:"versions"`
	GPUs              []CatalogGPU       `json:"gpus"`
}

// CatalogDevice is a handset, and the binding unit on mobile: one model fixes
// the screen, the ratio, the GPU, the cores and the memory together, because
// they shipped in one box. Only the release and which card answered are choices.
type CatalogDevice struct {
	Name     string    `json:"name"`
	OS       OS        `json:"os"`
	Browsers []Browser `json:"browsers"`
	// Models is what Sec-CH-UA-Model reports; several codes share one handset.
	Models              []string           `json:"models"`
	Versions            []CatalogOSVersion `json:"versions"`
	NavigatorPlatforms  []string           `json:"navigator_platforms"`
	Screen              Screen             `json:"screen"`
	HardwareConcurrency []uint32           `json:"hardware_concurrency"`
	Memory              []uint32           `json:"memory"`
	GPUs                []CatalogGPU       `json:"gpus"`
}

// CatalogOSVersion is one release and the UA-CH platform versions that belong
// to it, newest first. The two are different strings on purpose: Windows 11
// reports 15.0.0, and Windows 7 and 8 both report 0.0.0.
type CatalogOSVersion struct {
	OSVersion        string   `json:"os_version"`
	PlatformVersions []string `json:"platform_versions"`
}

// CatalogBrowser lists released versions of a browser, newest first.
type CatalogBrowser struct {
	Browser  Browser  `json:"browser"`
	Versions []string `json:"versions"`
}

// CatalogScreen is a logical resolution and the share of desktops reporting it.
type CatalogScreen struct {
	Width  uint32  `json:"width"`
	Height uint32  `json:"height"`
	Share  float32 `json:"share"`
}

// CatalogGPU is one card's WebGL strings. It says nothing about where it runs
// because it does not have to: it is listed under the system or the handset
// that reports it, and ANGLE over Direct3D on a Mac is not a card offered in the
// wrong place, it is a card that does not exist.
type CatalogGPU struct {
	Vendor        string `json:"vendor"`
	WebGLRenderer string `json:"webgl_renderer"`
	WebGLVendor   string `json:"webgl_vendor"`
}

// CatalogGeo ties a country to the language list and clock a browser there
// would report. Exit geography that disagrees with either is a known tell.
type CatalogGeo struct {
	Code      Geo      `json:"code"`
	Languages []string `json:"languages"`
	Timezone  string   `json:"timezone"`
}

var (
	catalogOnce  sync.Once
	catalogValue Catalog
	catalogErr   error
)

// IdentityCatalog decodes the embedded catalogue once and returns it.
func IdentityCatalog() (Catalog, error) {
	catalogOnce.Do(func() {
		if err := json.Unmarshal(catalogJSON, &catalogValue); err != nil {
			catalogErr = fmt.Errorf("decode browser identity catalog: %w", err)
		}
	})
	return catalogValue, catalogErr
}
