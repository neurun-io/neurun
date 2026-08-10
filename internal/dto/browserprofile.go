package dto

import (
	"slices"
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

type CreateBrowserProfileRequest struct {
	Name    string `json:"name"`
	Browser string `json:"browser"`
	// Identity absent launches the browser as itself.
	Identity *browser.Identity `json:"identity"`
}

type UpdateBrowserProfileRequest struct {
	Name *string `json:"name"`
	// Identity present replaces the presentation; a JSON null strips it. Absent
	// leaves it alone, which is why this needs two levels of pointer.
	Identity **browser.Identity `json:"identity"`
}

// BrowserProfileResponse never carries a secret. Cookie values and the identity
// proxy are what make a profile worth stealing, so they are summarised here and
// returned only by the state endpoint, which needs a write scope.
type BrowserProfileResponse struct {
	ID             string            `json:"id"`
	Name           string            `json:"name"`
	Browser        string            `json:"browser"`
	Identity       *IdentityResponse `json:"identity"`
	Cookies        []CookieResponse  `json:"cookies"`
	StorageOrigins []string          `json:"storage_origins"`
	CreatedAt      time.Time         `json:"created_at"`
	UpdatedAt      time.Time         `json:"updated_at"`
}

// IdentityResponse is the stored identity with the proxy URL removed. Its
// credentials are in that string, so it is reported as set or unset.
type IdentityResponse struct {
	browser.Identity
	Proxy    string `json:"proxy,omitempty"`
	ProxySet bool   `json:"proxy_set"`
}

// CookieResponse describes a cookie without disclosing it. A caller can see
// what a profile is logged into, and delete it, without reading the credential.
type CookieResponse struct {
	Name      string   `json:"name"`
	Domain    string   `json:"domain"`
	Path      string   `json:"path"`
	Expires   *float64 `json:"expires,omitempty"`
	Secure    bool     `json:"secure"`
	HTTPOnly  bool     `json:"http_only"`
	SameSite  string   `json:"same_site,omitempty"`
	ValueSize int      `json:"value_size"`
}

// BrowserProfileStateResponse is the unredacted state. Exporting it exports
// live session cookies, which is exporting credentials.
type BrowserProfileStateResponse struct {
	Cookies        []browser.Cookie `json:"cookies"`
	LocalStorage   browser.Storage  `json:"local_storage"`
	SessionStorage browser.Storage  `json:"session_storage"`
}

// SaveBrowserProfileStateRequest is what the SDK sends after closing a session.
// Absent collections mean empty, not unchanged: the browser hands back its whole
// jar, so this replaces rather than merges.
type SaveBrowserProfileStateRequest struct {
	Cookies        []browser.Cookie `json:"cookies"`
	LocalStorage   browser.Storage  `json:"local_storage"`
	SessionStorage browser.Storage  `json:"session_storage"`
}

func NewBrowserProfileResponse(record browser.Profile) BrowserProfileResponse {
	response := BrowserProfileResponse{
		ID:             record.ID,
		Name:           record.Name,
		Browser:        string(record.Browser),
		Cookies:        make([]CookieResponse, 0, len(record.Cookies)),
		StorageOrigins: storageOrigins(record),
		CreatedAt:      record.CreatedAt,
		UpdatedAt:      record.UpdatedAt,
	}
	if record.Identity != nil {
		identity := *record.Identity
		proxySet := identity.Proxy != ""
		identity.Proxy = ""
		response.Identity = &IdentityResponse{Identity: identity, ProxySet: proxySet}
	}
	for _, cookie := range record.Cookies {
		response.Cookies = append(response.Cookies, CookieResponse{
			Name: cookie.Name, Domain: cookie.Domain, Path: cookie.Path,
			Expires: cookie.Expires, Secure: cookie.Secure,
			HTTPOnly: cookie.HTTPOnly, SameSite: cookie.SameSite,
			ValueSize: len(cookie.Value),
		})
	}
	return response
}

func NewBrowserProfileResponses(records []browser.Profile) []BrowserProfileResponse {
	responses := make([]BrowserProfileResponse, len(records))
	for index, record := range records {
		responses[index] = NewBrowserProfileResponse(record)
	}
	return responses
}

func NewBrowserProfileStateResponse(record browser.Profile) BrowserProfileStateResponse {
	return BrowserProfileStateResponse{
		Cookies:        record.Cookies,
		LocalStorage:   record.LocalStorage,
		SessionStorage: record.SessionStorage,
	}
}

// storageOrigins names what a profile remembers, which is enough for a list
// view and discloses none of the values.
func storageOrigins(record browser.Profile) []string {
	seen := make(map[string]struct{}, len(record.LocalStorage)+len(record.SessionStorage))
	origins := make([]string, 0, len(seen))
	for _, storage := range []browser.Storage{record.LocalStorage, record.SessionStorage} {
		for origin := range storage {
			if _, exists := seen[origin]; exists {
				continue
			}
			seen[origin] = struct{}{}
			origins = append(origins, origin)
		}
	}
	slices.Sort(origins)
	return origins
}
