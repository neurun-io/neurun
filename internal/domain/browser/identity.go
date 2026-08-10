package browser

import (
	"fmt"
	"strings"
)

// Identity is the presentation half of a profile: who the browser claims to be.
// It mirrors the rustenium-identity record the browser server applies, and is
// stored as one jsonb document because that crate owns the schema.
type Identity struct {
	// DeviceModel empty means a desktop or laptop.
	DeviceModel         string   `json:"device_model,omitempty"`
	HasBattery          bool     `json:"has_battery"`
	HasMouse            bool     `json:"has_mouse"`
	HasTouch            bool     `json:"has_touch"`
	OS                  OS       `json:"os"`
	OSVersion           string   `json:"os_version"`
	Platform            Platform `json:"platform"`
	Brand               Brand    `json:"brand"`
	BrowserVersion      []uint32 `json:"browser_version"`
	Screen              Screen   `json:"screen"`
	HardwareConcurrency uint32   `json:"hardware_concurrency"`
	// Memory is deviceMemory in GiB.
	Memory       uint32   `json:"memory"`
	GPU          GPU      `json:"gpu"`
	Geo          Geo      `json:"geo"`
	Language     []string `json:"language"`
	HistoryCount *uint32  `json:"history_count,omitempty"`
	// Proxy is a full URL with credentials. It is a secret and must be redacted
	// before an identity leaves the API.
	Proxy string `json:"proxy,omitempty"`
	// Timezone is an IANA name. Empty means resolve it through the proxy.
	Timezone string `json:"timezone,omitempty"`
}

type Platform struct {
	Bitness      string `json:"bitness,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	// NavigatorPlatform is one of the known values, or any other string.
	NavigatorPlatform string `json:"navigator_platform"`
	Version           string `json:"version"`
}

type Screen struct {
	LogicalWidth      uint32  `json:"logical_width"`
	LogicalHeight     uint32  `json:"logical_height"`
	OriginalWidth     uint32  `json:"original_width"`
	OriginalHeight    uint32  `json:"original_height"`
	DensityPixelRatio float32 `json:"density_pixel_ratio"`
}

type GPU struct {
	Vendor        string `json:"vendor"`
	WebGLRenderer string `json:"webgl_renderer"`
	WebGLVendor   string `json:"webgl_vendor"`
}

type OS string

const (
	OSWindows   OS = "Windows"
	OSMacintosh OS = "Macintosh"
	OSLinux     OS = "Linux"
	OSAndroid   OS = "Android"
	OSIOS       OS = "Ios"
)

func (value OS) Valid() bool {
	switch value {
	case OSWindows, OSMacintosh, OSLinux, OSAndroid, OSIOS:
		return true
	default:
		return false
	}
}

// Brand is the browser an identity claims to be, which is not the browser it
// runs on: a Chrome process can present as Edge. There is no Firefox brand
// because rustenium-identity has none.
type Brand string

const (
	BrandChrome Brand = "chrome"
	BrandSafari Brand = "safari"
	BrandEdge   Brand = "edge"
)

func (value Brand) Valid() bool {
	switch value {
	case BrandChrome, BrandSafari, BrandEdge:
		return true
	default:
		return false
	}
}

// The navigator.platform values the browser server maps to a known variant.
// Anything else travels as an opaque string.
const (
	PlatformWin32       = "Win32"
	PlatformMacIntel    = "MacIntel"
	PlatformLinuxX8664  = "Linux x86_64"
	PlatformLinuxArmV81 = "Linux armv81"
	PlatformIPhone      = "iPhone"
)

type Geo string

func (value Geo) Valid() bool {
	switch value {
	case "US", "UK", "JP", "DE", "FR", "CA", "AU", "IN", "BR",
		"KR", "IT", "ES", "NL", "PL", "SE", "MX", "SG", "ZA":
		return true
	default:
		return false
	}
}

func (record Identity) Validate() error {
	if !record.OS.Valid() {
		return fmt.Errorf("%w: identity os is invalid", ErrInvalid)
	}
	if !record.Brand.Valid() {
		return fmt.Errorf("%w: identity brand is invalid", ErrInvalid)
	}
	if !record.Geo.Valid() {
		return fmt.Errorf("%w: identity geo is invalid", ErrInvalid)
	}
	if strings.TrimSpace(record.OSVersion) == "" {
		return fmt.Errorf("%w: identity requires an os version", ErrInvalid)
	}
	if strings.TrimSpace(record.Platform.NavigatorPlatform) == "" ||
		strings.TrimSpace(record.Platform.Version) == "" {
		return fmt.Errorf("%w: identity platform is incomplete", ErrInvalid)
	}
	if len(record.BrowserVersion) == 0 {
		return fmt.Errorf("%w: identity requires a browser version", ErrInvalid)
	}
	if len(record.Language) == 0 {
		return fmt.Errorf("%w: identity requires at least one language", ErrInvalid)
	}
	if record.HardwareConcurrency == 0 || record.Memory == 0 {
		return fmt.Errorf("%w: identity hardware is invalid", ErrInvalid)
	}
	if record.Screen.LogicalWidth == 0 || record.Screen.LogicalHeight == 0 ||
		record.Screen.OriginalWidth == 0 || record.Screen.OriginalHeight == 0 ||
		record.Screen.DensityPixelRatio <= 0 {
		return fmt.Errorf("%w: identity screen is invalid", ErrInvalid)
	}
	if record.GPU.Vendor == "" || record.GPU.WebGLRenderer == "" ||
		record.GPU.WebGLVendor == "" {
		return fmt.Errorf("%w: identity gpu is incomplete", ErrInvalid)
	}
	return nil
}
