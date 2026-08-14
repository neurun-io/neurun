package browser

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Identity is the presentation half of a profile: who the browser claims to be.
// It mirrors the rustenium-identity record the browser server applies, and is
// stored as one jsonb document because that crate owns the schema.
type Identity struct {
	// DeviceModel empty means a desktop or laptop.
	DeviceModel string   `json:"device_model,omitempty"`
	HasBattery  bool     `json:"has_battery"`
	HasMouse    bool     `json:"has_mouse"`
	HasTouch    bool     `json:"has_touch"`
	OS          OS       `json:"os"`
	OSVersion   string   `json:"os_version"`
	Platform    Platform `json:"platform"`
	// Browser is what it claims to be, which is not what runs: rustenium-identity
	// drives Chrome, so a Safari profile is a Chrome wearing Safari.
	Browser             Browser  `json:"browser"`
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
	if !record.Browser.Valid() {
		return fmt.Errorf("%w: identity browser is invalid", ErrInvalid)
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

// DefaultIdentity is what a profile wears when its owner does not choose. Every
// profile has one, because a browser presenting as itself is the easiest kind to
// catch.
//
// It is drawn from the catalogue and seeded: coherent, because nothing is
// invented; stable, because the same seed picks the same machine and an identity
// that moves between runs is its own tell; and different from the next seed's,
// because one canned persona shared by a whole fleet makes the fleet obvious.
//
// An empty claimed leaves the browser to the draw as well.
func DefaultIdentity(seed string, claimed Browser) (Identity, error) {
	catalog, err := IdentityCatalog()
	if err != nil {
		return Identity{}, err
	}
	next := picker(seed)

	// Desktops only. A default that landed on a handset would hand back a
	// touch-only session nobody asked for.
	systems := make([]CatalogOS, 0, len(catalog.OperatingSystems))
	for _, system := range catalog.OperatingSystems {
		if system.FormFactor != "desktop" ||
			len(system.Versions) == 0 || len(system.Browsers) == 0 {
			continue
		}
		if claimed != "" && !slices.Contains(system.Browsers, claimed) {
			continue
		}
		systems = append(systems, system)
	}
	if len(systems) == 0 || len(catalog.Screens) == 0 ||
		len(catalog.DensityPixelRatios) == 0 || len(catalog.Geos) == 0 ||
		len(catalog.HardwareConcurrency) == 0 || len(catalog.Memory) == 0 {
		return Identity{}, fmt.Errorf(
			"%w: the identity catalog offers no %s desktop", ErrInvalid, claimed,
		)
	}

	system := systems[next(len(systems))]
	release := system.Versions[next(len(system.Versions))]
	if claimed == "" {
		claimed = system.Browsers[next(len(system.Browsers))]
	}
	screen := catalog.Screens[next(len(catalog.Screens))]
	ratio := catalog.DensityPixelRatios[next(len(catalog.DensityPixelRatios))]
	geo := catalog.Geos[next(len(catalog.Geos))]

	versions := browserVersionsFor(catalog, claimed)
	if len(system.GPUs) == 0 || len(versions) == 0 {
		return Identity{}, fmt.Errorf(
			"%w: the identity catalog offers no %s on %s", ErrInvalid, claimed, system.OS,
		)
	}
	gpu := system.GPUs[next(len(system.GPUs))]

	identity := Identity{
		HasMouse:  true,
		OS:        system.OS,
		OSVersion: release.OSVersion,
		Platform: Platform{
			Bitness:           system.Bitness,
			Architecture:      system.Architecture,
			NavigatorPlatform: system.NavigatorPlatform,
			Version:           release.PlatformVersions[next(len(release.PlatformVersions))],
		},
		Browser:        claimed,
		BrowserVersion: versionNumbers(versions[next(len(versions))]),
		Screen: Screen{
			LogicalWidth:      screen.Width,
			LogicalHeight:     screen.Height,
			OriginalWidth:     uint32(float32(screen.Width) * ratio),
			OriginalHeight:    uint32(float32(screen.Height) * ratio),
			DensityPixelRatio: ratio,
		},
		HardwareConcurrency: catalog.HardwareConcurrency[next(len(catalog.HardwareConcurrency))],
		Memory:              catalog.Memory[next(len(catalog.Memory))],
		GPU: GPU{
			Vendor:        gpu.Vendor,
			WebGLRenderer: gpu.WebGLRenderer,
			WebGLVendor:   gpu.WebGLVendor,
		},
		Geo:      geo.Code,
		Language: geo.Languages,
		Timezone: geo.Timezone,
	}
	return identity, identity.Validate()
}

// EphemeralIdentity is the persona for a session that names no profile: a
// coherent machine drawn fresh and thrown away when the session ends.
//
// Nothing about it is stable across runs, which is a cost, not a feature — it is
// the price of not keeping the persona anywhere. A caller that wants the same
// machine tomorrow wants a profile.
func EphemeralIdentity(claimed Browser) (Identity, error) {
	seed := make([]byte, 16)
	if _, err := rand.Read(seed); err != nil {
		return Identity{}, fmt.Errorf("seed an ephemeral identity: %w", err)
	}
	return DefaultIdentity(string(seed), claimed)
}

func browserVersionsFor(catalog Catalog, claimed Browser) []string {
	for _, entry := range catalog.Browsers {
		if entry.Browser == claimed {
			return entry.Versions
		}
	}
	return nil
}

// picker draws indices from a seed. The same seed picks the same identity for
// as long as the catalogue holds still, which is the point: a persona that
// changes between runs is louder than a known-bad one.
func picker(seed string) func(length int) int {
	digest := sha256.Sum256([]byte(seed))
	state := binary.BigEndian.Uint64(digest[:8])
	return func(length int) int {
		if length <= 0 {
			return 0
		}
		state = state*6364136223846793005 + 1442695040888963407
		return int((state >> 33) % uint64(length))
	}
}

// versionNumbers splits 139.0.6889.109, and Safari's bare 18, into the parts the
// record carries.
func versionNumbers(version string) []uint32 {
	parts := strings.Split(version, ".")
	numbers := make([]uint32, 0, len(parts))
	for _, part := range parts {
		value, err := strconv.ParseUint(strings.TrimSpace(part), 10, 32)
		if err != nil {
			continue
		}
		numbers = append(numbers, uint32(value))
	}
	return numbers
}
