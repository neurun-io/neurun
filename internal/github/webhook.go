package github

import (
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-github/v88/github"
)

var (
	ErrNoWebhookSecret = errors.New("github webhook secret is not configured")
	ErrSignature       = errors.New("github delivery signature is not valid")
)

// zeroCommit is the after a push carries when the ref was deleted rather than
// advanced.
const zeroCommit = "0000000000000000000000000000000000000000"

// Push is the part of a push delivery this control plane acts on. The
// installation is the only thing tying it to an organization, and GitHub signed
// it, so nothing here is taken from a caller.
type Push struct {
	InstallationID int64
	Repository     string
	Ref            string
	Commit         string
	DefaultBranch  string
	Delivery       string
}

// ParsePush verifies a delivery against the app's webhook secret and returns
// the push it carries. A signed delivery that is not a push, or is a push with
// nothing to build, reports false rather than an error: it is a delivery the
// server correctly has no work for.
func (client *Client) ParsePush(request *http.Request) (Push, bool, error) {
	if len(client.webhookSecret) == 0 {
		return Push{}, false, ErrNoWebhookSecret
	}
	payload, err := github.ValidatePayload(request, client.webhookSecret)
	if err != nil {
		// Both verbs are %w so a body that ran past its limit stays inspectable
		// underneath the rejection.
		return Push{}, false, fmt.Errorf("%w: %w", ErrSignature, err)
	}
	if github.WebHookType(request) != "push" {
		return Push{}, false, nil
	}
	parsed, err := github.ParseWebHook("push", payload)
	if err != nil {
		return Push{}, false, fmt.Errorf("github: parse push delivery: %w", err)
	}
	event, ok := parsed.(*github.PushEvent)
	if !ok {
		return Push{}, false, errors.New("github: push delivery did not decode as a push")
	}

	push := Push{
		InstallationID: event.GetInstallation().GetID(),
		Repository:     event.GetRepo().GetFullName(),
		Ref:            event.GetRef(),
		Commit:         event.GetAfter(),
		DefaultBranch:  event.GetRepo().GetDefaultBranch(),
		Delivery:       github.DeliveryID(request),
	}
	if event.GetDeleted() || push.InstallationID <= 0 ||
		push.Repository == "" || push.Ref == "" || !isCommitSHA(push.Commit) {
		return push, false, nil
	}
	return push, true, nil
}

func isCommitSHA(value string) bool {
	if len(value) != 40 || value == zeroCommit {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}
