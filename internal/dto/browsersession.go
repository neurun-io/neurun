package dto

import (
	"time"

	"github.com/neurun-io/neurun/internal/domain/browser"
)

// BrowserSessionResponse is a live session as an operator sees it. The display
// address is deliberately absent: a viewer asks the control plane to stream and
// never learns where the framebuffer actually is.
type BrowserSessionResponse struct {
	ID          string    `json:"id"`
	AppID       string    `json:"app_id"`
	ExecutionID string    `json:"execution_id,omitempty"`
	ProfileID   string    `json:"browser_profile_id,omitempty"`
	Browser     string    `json:"browser"`
	Status      string    `json:"status"`
	StartedAt   time.Time `json:"started_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func NewBrowserSessionResponse(record browser.Session) BrowserSessionResponse {
	return BrowserSessionResponse{
		ID: record.ID, AppID: record.AppID, ExecutionID: record.ExecutionID,
		ProfileID: record.ProfileID, Browser: string(record.Browser),
		Status:    string(record.Status),
		StartedAt: record.StartedAt, UpdatedAt: record.UpdatedAt,
	}
}

func NewBrowserSessionResponses(records []browser.Session) []BrowserSessionResponse {
	responses := make([]BrowserSessionResponse, len(records))
	for index, record := range records {
		responses[index] = NewBrowserSessionResponse(record)
	}
	return responses
}

// OpenBrowserSessionRequest opens a browser for a caller holding an API key.
//
// There is no app and no execution: an API key belongs to an organization, not
// to a run, and the session it opens says so by naming neither.
type OpenBrowserSessionRequest struct {
	// chrome or safari. Read only when no profile is named, because a profile's
	// identity already says which browser it is.
	Browser string `json:"browser"`
	// The profile to wear. Absent still gets a persona — one is drawn for the
	// session — it just is not kept, along with the cookies it collected.
	ProfileID string `json:"browser_profile_id,omitempty"`
	// Start from what the profile remembers. Needs a profile to read from.
	LoadStorage bool `json:"load_storage,omitempty"`
}

// CloseBrowserSessionRequest is the body a DELETE may carry.
type CloseBrowserSessionRequest struct {
	// Capture what the browser holds into the profile it wears, replacing what
	// was there rather than merging into it. Needs a profile.
	SaveStorage bool `json:"save_storage,omitempty"`
}

type NavigateBrowserRequest struct {
	URL string `json:"url"`
	// Absent sends no Referer, which is not the same as sending an empty one.
	Referer *string `json:"referer,omitempty"`
}

type WaitForBrowserNavigationRequest struct {
	// commit, dom_content_loaded, load, network_almost_idle or network_idle.
	// Absent is load, the browser's own default.
	WaitUntil string `json:"wait_until,omitempty"`
	// Zero leaves the browser's own timeout in place.
	TimeoutMs uint32 `json:"timeout_ms,omitempty"`
}

// BrowserNodeResponse is one element as the browser found it.
type BrowserNodeResponse struct {
	NodeID int64 `json:"node_id"`
	// The tag, as the DOM spells a local name: lower case for HTML.
	LocalName string `json:"local_name"`
	// The DOM's own numbering: 1 is an element, 3 is text.
	NodeType   uint32            `json:"node_type"`
	Attributes map[string]string `json:"attributes"`
	// What a reader would see, which is not the same as what the markup holds.
	Text string `json:"text"`
	HTML string `json:"html"`
	// The bounding box, in viewport coordinates, at the moment of the read. A
	// scroll moves it, so it is worth no more than the instant it was taken.
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// BrowserPointerRequest aims a pointer command.
//
// A selector wins over x and y: an element knows where it is, and a caller
// holding a rectangle from before the last scroll does not. Neither, on a
// click, means where the pointer already is.
type BrowserPointerRequest struct {
	Selector string   `json:"selector,omitempty"`
	X        *float64 `json:"x,omitempty"`
	Y        *float64 `json:"y,omitempty"`
}

type ClickBrowserRequest struct {
	BrowserPointerRequest
	// left, middle, right, back or forward. Absent is left.
	Button string `json:"button,omitempty"`
	// Zero is one click. Two is a double click, with a pause between that is
	// drawn rather than fixed.
	Count uint32 `json:"count,omitempty"`
	// How long the button is held. Zero leaves it to be drawn per click, which
	// is the point — a press that is always the same length is the tell.
	DelayMs uint32 `json:"delay_ms,omitempty"`
}

type TypeIntoBrowserRequest struct {
	Text string `json:"text"`
	// An element to type into. It is clicked, humanly, and the click is what
	// focuses it.
	Selector string `json:"selector,omitempty"`
	// How long each key is held, drawn fresh from this range; the gap before the
	// next key follows from it. Both zero is an average typist.
	DelayMinMs uint32 `json:"delay_min_ms,omitempty"`
	DelayMaxMs uint32 `json:"delay_max_ms,omitempty"`
}

type ScrollBrowserRequest struct {
	// Down is positive. Only the y axis: the browser's own scroll takes an x
	// distance and drops it.
	DeltaY int32 `json:"delta_y"`
}

type ScrollBrowserToRequest struct {
	Selector string `json:"selector"`
	// top, center or bottom — where in the viewport the element comes to rest.
	// Absent is center, which is where a person scrolls something to look at it.
	Align string `json:"align,omitempty"`
}

// BrowserCookiesRequest is a whole jar going into a session.
type BrowserCookiesRequest struct {
	Cookies []browser.Cookie `json:"cookies"`
}

// BrowserCookiesResponse is a whole jar coming out of one. Values are not
// redacted the way a profile's are: this is the endpoint whose entire purpose
// is handing them back, which is why it takes the write scope.
type BrowserCookiesResponse struct {
	Cookies []browser.Cookie `json:"cookies"`
}
