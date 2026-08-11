package browser

import (
	"errors"
	"slices"
	"testing"
)

func TestCatalogDecodes(t *testing.T) {
	t.Parallel()

	catalog, err := IdentityCatalog()
	if err != nil {
		t.Fatalf("IdentityCatalog: %v", err)
	}
	if len(catalog.OperatingSystems) == 0 || len(catalog.Browsers) == 0 ||
		len(catalog.Screens) == 0 || len(catalog.GPUs) == 0 || len(catalog.Geos) == 0 {
		t.Fatal("catalog is missing a section")
	}
}

// Every value the catalogue offers has to survive the same validation a caller's
// identity does, or the catalogue is offering choices the API will refuse.
func TestCatalogOffersOnlyValidValues(t *testing.T) {
	t.Parallel()

	catalog, _ := IdentityCatalog()

	for _, entry := range catalog.OperatingSystems {
		if !entry.OS.Valid() {
			t.Errorf("os %q is invalid", entry.OS)
		}
		if len(entry.Brands) == 0 {
			t.Errorf("os %q offers no browser", entry.OS)
		}
		// A phone OS carries neither platform nor releases: the handset does.
		if entry.FormFactor == "desktop" &&
			(entry.NavigatorPlatform == "" || len(entry.Versions) == 0) {
			t.Errorf("os %q is incomplete", entry.OS)
		}
		if entry.FormFactor == "mobile" && len(devicesFor(catalog, entry.OS)) == 0 {
			t.Errorf("os %q has no devices to carry it", entry.OS)
		}
		for _, brand := range entry.Brands {
			if !brand.Valid() {
				t.Errorf("os %q offers invalid brand %q", entry.OS, brand)
			}
		}
		for _, version := range entry.Versions {
			if version.OSVersion == "" || len(version.PlatformVersions) == 0 {
				t.Errorf("os %q version %q is incomplete", entry.OS, version.OSVersion)
			}
		}
	}

	for _, entry := range catalog.Browsers {
		if !entry.Brand.Valid() || len(entry.Versions) == 0 {
			t.Errorf("browser %q is invalid", entry.Brand)
		}
	}

	for _, entry := range catalog.Geos {
		if !entry.Code.Valid() || len(entry.Languages) == 0 || entry.Timezone == "" {
			t.Errorf("geo %q is incomplete", entry.Code)
		}
	}
}

// Every handset has to be able to produce a whole identity on its own, because
// choosing one is what fills the screen, card, cores and memory. A device
// missing any of them would offer a profile the API then refuses.
func TestCatalogDevicesAreComplete(t *testing.T) {
	t.Parallel()

	catalog, _ := IdentityCatalog()
	if len(catalog.Devices) == 0 {
		t.Fatal("catalog has no devices")
	}

	for _, device := range catalog.Devices {
		if !device.OS.Valid() || device.Name == "" {
			t.Errorf("device %q is unnamed or on an invalid os", device.Name)
		}
		if len(device.Models) == 0 || len(device.Versions) == 0 ||
			len(device.NavigatorPlatforms) == 0 || len(device.GPUs) == 0 ||
			len(device.HardwareConcurrency) == 0 || len(device.Memory) == 0 ||
			len(device.Brands) == 0 {
			t.Errorf("device %q is incomplete", device.Name)
			continue
		}
		if device.Screen.LogicalWidth == 0 || device.Screen.LogicalHeight == 0 ||
			device.Screen.OriginalWidth == 0 || device.Screen.OriginalHeight == 0 ||
			device.Screen.DensityPixelRatio <= 0 {
			t.Errorf("device %q has an invalid screen", device.Name)
		}
		for _, version := range device.Versions {
			if version.OSVersion == "" || len(version.PlatformVersions) == 0 {
				t.Errorf("device %q release %q is incomplete", device.Name, version.OSVersion)
			}
		}
		for _, gpu := range device.GPUs {
			if gpu.Vendor == "" || gpu.WebGLRenderer == "" || gpu.WebGLVendor == "" {
				t.Errorf("device %q has an incomplete gpu", device.Name)
			}
		}
		// deviceMemory is a power of two the browser caps at 8; installed RAM is
		// not what navigator returns.
		for _, memory := range device.Memory {
			if memory == 0 || memory > 8 || memory&(memory-1) != 0 {
				t.Errorf("device %q reports %d GiB, which no browser does", device.Name, memory)
			}
		}
	}
}

// A GPU is only coherent on the platform that reports it: ANGLE over Direct3D
// is Windows, and Safari reports its own pair whatever card is underneath.
func TestCatalogBindsGPUsToTheirPlatform(t *testing.T) {
	t.Parallel()

	catalog, _ := IdentityCatalog()
	brands := map[OS]map[Brand]bool{}
	for _, entry := range catalog.OperatingSystems {
		brands[entry.OS] = map[Brand]bool{}
		for _, brand := range entry.Brands {
			brands[entry.OS][brand] = true
		}
	}

	for _, gpu := range catalog.GPUs {
		if !gpu.OS.Valid() {
			t.Errorf("gpu %q names invalid os %q", gpu.WebGLRenderer, gpu.OS)
			continue
		}
		if gpu.Vendor == "" || gpu.WebGLRenderer == "" || gpu.WebGLVendor == "" {
			t.Errorf("gpu %q is incomplete", gpu.WebGLRenderer)
		}
		for _, brand := range gpu.Brands {
			if !brands[gpu.OS][brand] {
				t.Errorf("gpu %q offers %q, which does not run on %q",
					gpu.WebGLRenderer, brand, gpu.OS)
			}
		}
	}
}

// The point of the catalogue: an identity assembled from it, and nothing else,
// is one the domain accepts.
func TestCatalogAssemblesAValidIdentity(t *testing.T) {
	t.Parallel()

	catalog, _ := IdentityCatalog()

	geo := catalog.Geos[0]

	for _, system := range catalog.OperatingSystems {
		for _, brand := range system.Brands {
			identity, err := assemble(catalog, system, brand)
			if err != nil {
				t.Errorf("%s on %s: %v", brand, system.OS, err)
				continue
			}
			identity.Geo = geo.Code
			identity.Language = geo.Languages
			identity.Timezone = geo.Timezone
			if err := identity.Validate(); err != nil {
				t.Errorf("%s on %s: %v", brand, system.OS, err)
			}
		}
	}
}

// assemble builds the identity the form would: from the OS on a desktop, and
// from a handset on mobile, where one device fixes screen, GPU and memory.
func assemble(catalog Catalog, system CatalogOS, brand Brand) (Identity, error) {
	identity := Identity{
		OS:             system.OS,
		Brand:          brand,
		BrowserVersion: []uint32{1, 0, 0, 0},
	}

	if system.FormFactor == "mobile" {
		devices := devicesFor(catalog, system.OS)
		if len(devices) == 0 {
			return identity, errors.New("no devices")
		}
		device := devices[0]
		if len(device.GPUs) == 0 || len(device.Versions) == 0 ||
			len(device.NavigatorPlatforms) == 0 {
			return identity, errors.New("device is incomplete")
		}
		identity.OSVersion = device.Versions[0].OSVersion
		identity.Platform = Platform{
			NavigatorPlatform: device.NavigatorPlatforms[0],
			Version:           device.Versions[0].PlatformVersions[0],
		}
		identity.DeviceModel = device.Models[0]
		identity.Screen = device.Screen
		identity.HardwareConcurrency = device.HardwareConcurrency[0]
		identity.Memory = device.Memory[0]
		identity.GPU = GPU{
			Vendor:        device.GPUs[0].Vendor,
			WebGLRenderer: device.GPUs[0].WebGLRenderer,
			WebGLVendor:   device.GPUs[0].WebGLVendor,
		}
		return identity, nil
	}

	gpu, ok := firstGPU(catalog, system.OS, brand)
	if !ok {
		return identity, errors.New("no gpu")
	}
	version := system.Versions[0]
	screen := catalog.Screens[0]
	ratio := catalog.DensityPixelRatios[0]

	identity.OSVersion = version.OSVersion
	identity.Platform = Platform{
		Bitness:           system.Bitness,
		Architecture:      system.Architecture,
		NavigatorPlatform: system.NavigatorPlatform,
		Version:           version.PlatformVersions[0],
	}
	identity.Screen = Screen{
		LogicalWidth:      screen.Width,
		LogicalHeight:     screen.Height,
		OriginalWidth:     uint32(float32(screen.Width) * ratio),
		OriginalHeight:    uint32(float32(screen.Height) * ratio),
		DensityPixelRatio: ratio,
	}
	identity.HardwareConcurrency = catalog.HardwareConcurrency[0]
	identity.Memory = catalog.Memory[0]
	identity.GPU = GPU{
		Vendor:        gpu.Vendor,
		WebGLRenderer: gpu.WebGLRenderer,
		WebGLVendor:   gpu.WebGLVendor,
	}
	return identity, nil
}

func devicesFor(catalog Catalog, system OS) []CatalogDevice {
	var found []CatalogDevice
	for _, device := range catalog.Devices {
		if device.OS == system {
			found = append(found, device)
		}
	}
	return found
}

func firstGPU(catalog Catalog, system OS, brand Brand) (CatalogGPU, bool) {
	for _, gpu := range catalog.GPUs {
		if gpu.OS == system && slices.Contains(gpu.Brands, brand) {
			return gpu, true
		}
	}
	return CatalogGPU{}, false
}
