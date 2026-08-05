package api

import (
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/neurun-io/neurun/internal/domain/operator"
	"github.com/neurun-io/neurun/internal/domain/organization"
	"github.com/neurun-io/neurun/internal/dto"
)

// register creates an account and signs it in.
//
// Sign-up is open, and takes one of two shapes: name an organization and own
// it, or present an invitation and join one. Per-IP limiting belongs at the
// edge, not here.
func (server *Server) register(ctx *gin.Context) {
	if server.operators == nil || server.organizations == nil {
		writeProblem(ctx, http.StatusServiceUnavailable, dto.Problem{
			Code:    "registration_unavailable",
			Message: "registration is not configured on this server",
		})
		return
	}
	var body dto.RegisterRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	email := strings.TrimSpace(body.Email)
	organizationName := strings.TrimSpace(body.OrganizationName)
	inviteToken := strings.TrimSpace(body.InviteToken)

	if email == "" || body.Password == "" {
		invalidQuery(ctx, "email and password are required")
		return
	}
	if organizationName != "" && inviteToken != "" {
		invalidQuery(ctx,
			"supply organization_name to start an organization, or invite_token to join one, not both")
		return
	}

	record, err := server.accounts.CreateUser(ctx.Request.Context(), dto.CreateUserRequest{
		Email: email, Password: body.Password,
	})
	if err != nil {
		writeError(ctx, err)
		return
	}

	var membership organization.Member
	if organizationName != "" || inviteToken != "" {
		membership, err = server.joinOrCreate(ctx, record.ID, email, organizationName, inviteToken)
		if err != nil {
			// The account was made for an organization it could not reach. Undo
			// it rather than stranding a login somebody meant to attach.
			if deleteErr := server.accounts.DeleteUser(
				ctx.Request.Context(), record.ID,
			); deleteErr != nil {
				slog.Error("could not roll back a half-finished registration",
					"user_id", record.ID, "error", deleteErr)
			}
			writeError(ctx, err)
			return
		}
	}

	payload := gin.H{
		"user":       dto.NewUserResponse(record),
		"request_id": requestIDOf(ctx),
	}
	if membership.OrganizationID != "" {
		payload["member"] = dto.NewMemberResponse(membership)
		if found, err := server.organizations.Get(
			ctx.Request.Context(), membership.OrganizationID,
		); err == nil {
			payload["organization"] = dto.NewOrganizationResponse(found)
		}
	}

	session, token, err := server.operators.StartSession(
		ctx.Request.Context(), record.ID, membership,
	)
	if err == nil {
		http.SetCookie(ctx.Writer, server.sessionCookie(token, session.ExpiresAt))
		payload["operator"] = dto.NewOperatorResponse(session)
	}
	ctx.JSON(http.StatusCreated, payload)
}

func (server *Server) joinOrCreate(
	ctx *gin.Context,
	userID string,
	email string,
	organizationName string,
	inviteToken string,
) (organization.Member, error) {
	if inviteToken != "" {
		return server.organizations.Accept(
			ctx.Request.Context(), inviteToken, userID, email,
		)
	}
	created, err := server.organizations.Create(
		ctx.Request.Context(), userID, organizationName,
	)
	if err != nil {
		return organization.Member{}, err
	}
	return server.organizations.Membership(ctx.Request.Context(), created.ID, userID)
}

// lookupInvite lets the sign-up page name the organization somebody is about to
// join, without spending the token.
func (server *Server) lookupInvite(ctx *gin.Context) {
	token := strings.TrimSpace(ctx.Query("token"))
	if token == "" {
		invalidQuery(ctx, "token is required")
		return
	}
	invite, err := server.organizations.InviteByToken(ctx.Request.Context(), token)
	if err != nil {
		writeError(ctx, err)
		return
	}
	if err := invite.Redeemable(invite.Email, time.Now().UTC()); err != nil {
		writeError(ctx, err)
		return
	}
	found, err := server.organizations.Get(ctx.Request.Context(), invite.OrganizationID)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"organization": dto.NewOrganizationResponse(found),
		"email":        invite.Email,
		"role":         string(invite.Role),
	})
}

// createOrganization is how an account with no organization gets one. The
// session is re-issued on the spot, so the caller does not have to sign in
// again to use what they just made.
func (server *Server) createOrganization(ctx *gin.Context) {
	var body dto.CreateProjectRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	principal := principalOf(ctx)
	created, err := server.organizations.Create(
		ctx.Request.Context(), principal.OperatorID, body.Name,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	membership, err := server.organizations.Membership(
		ctx.Request.Context(), created.ID, principal.OperatorID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	server.reissue(ctx, principal.OperatorID, membership)
	ctx.Header("Location", "/v1/organization")
	ctx.JSON(http.StatusCreated, dto.NewOrganizationResponse(created))
}

// reissue swaps the caller's session for one scoped to a membership they have
// just gained. A failure here costs only convenience: the membership stands and
// the next sign-in picks it up.
func (server *Server) reissue(
	ctx *gin.Context,
	userID string,
	membership organization.Member,
) {
	if server.operators == nil {
		return
	}
	if token := sessionToken(ctx); token != "" {
		_ = server.operators.Logout(ctx.Request.Context(), token)
	}
	session, token, err := server.operators.StartSession(
		ctx.Request.Context(), userID, membership,
	)
	if err != nil {
		slog.Error("could not re-issue a session after a membership change",
			"user_id", userID, "error", err)
		return
	}
	http.SetCookie(ctx.Writer, server.sessionCookie(token, session.ExpiresAt))
}

func (server *Server) listOrganizations(ctx *gin.Context) {
	records, err := server.organizations.ListForUser(
		ctx.Request.Context(), principalOf(ctx).OperatorID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{
		"organizations": dto.NewOrganizationResponses(records),
	})
}

func (server *Server) getOrganization(ctx *gin.Context) {
	record, err := server.organizations.Get(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewOrganizationResponse(record))
}

func (server *Server) updateOrganization(ctx *gin.Context) {
	var body dto.CreateProjectRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.organizations.Rename(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, body.Name,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewOrganizationResponse(record))
}

func (server *Server) listMembers(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.organizations.ListMembers(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"members": dto.NewMemberResponses(records)})
}

func (server *Server) updateMember(ctx *gin.Context) {
	var body dto.UpdateMemberRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	record, err := server.organizations.SetMemberRole(
		ctx.Request.Context(), principalOf(ctx).OrganizationID,
		ctx.Param("user_id"), body.Role,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, dto.NewMemberResponse(record))
}

func (server *Server) removeMember(ctx *gin.Context) {
	principal := principalOf(ctx)
	target := ctx.Param("user_id")
	if target == principal.OperatorID {
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "cannot_remove_self",
			Message: "you cannot remove your own membership",
		})
		return
	}
	err := server.organizations.RemoveMember(
		ctx.Request.Context(), principal.OrganizationID, target,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

func (server *Server) listInvites(ctx *gin.Context) {
	limit, ok := server.pageLimit(ctx)
	if !ok {
		return
	}
	records, err := server.organizations.ListInvites(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, limit,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.JSON(http.StatusOK, gin.H{"invites": dto.NewInviteResponses(records)})
}

func (server *Server) createInvite(ctx *gin.Context) {
	var body dto.InviteRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	principal := principalOf(ctx)
	record, err := server.organizations.Invite(
		ctx.Request.Context(), principal.OrganizationID, principal.OperatorID, body,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Header("Location", "/v1/invites/"+record.ID)
	ctx.JSON(http.StatusCreated, dto.CreatedInviteResponse{
		InviteResponse: dto.NewInviteResponse(record.Invite),
		Token:          record.Token,
	})
}

func (server *Server) revokeInvite(ctx *gin.Context) {
	err := server.organizations.RevokeInvite(
		ctx.Request.Context(), principalOf(ctx).OrganizationID, ctx.Param("invite_id"),
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	ctx.Status(http.StatusNoContent)
}

// acceptInvite joins an existing account to another organization. The session
// stays where it is; signing in again lands on the new membership only if it is
// the oldest one, so the response names what was joined.
func (server *Server) acceptInvite(ctx *gin.Context) {
	var body dto.AcceptInviteRequest
	if !server.bindJSON(ctx, &body) {
		return
	}
	principal := principalOf(ctx)
	record, err := server.organizations.Accept(
		ctx.Request.Context(), body.Token, principal.OperatorID, principal.Email,
	)
	if err != nil {
		writeError(ctx, err)
		return
	}
	// An account with nowhere to act has just gained somewhere. Move its session
	// there rather than making it sign in again.
	if principal.OrganizationID == "" {
		server.reissue(ctx, principal.OperatorID, record)
	}
	ctx.JSON(http.StatusOK, dto.NewMemberResponse(record))
}

// writeOrganizationError maps tenancy failures onto their HTTP shape.
func writeOrganizationError(ctx *gin.Context, err error) bool {
	switch {
	case errors.Is(err, organization.ErrNotFound):
		notFound(ctx, "resource")
	case errors.Is(err, organization.ErrNotMember):
		writeProblem(ctx, http.StatusForbidden, dto.Problem{
			Code:    "not_a_member",
			Message: "the caller is not a member of this organization",
		})
	case errors.Is(err, organization.ErrAlreadyOwner):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "already_owns_an_organization",
			Message: "an account may own one organization, and join any number",
		})
	case errors.Is(err, organization.ErrOwnerLocked):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "owner_locked",
			Message: "the owner's membership cannot be changed or removed",
		})
	case errors.Is(err, organization.ErrInviteSpent):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "invite_spent",
			Message: "this invitation has already been accepted or revoked",
		})
	case errors.Is(err, organization.ErrInviteStale):
		writeProblem(ctx, http.StatusGone, dto.Problem{
			Code:    "invite_expired",
			Message: "this invitation has expired",
		})
	case errors.Is(err, organization.ErrInviteAddress):
		writeProblem(ctx, http.StatusForbidden, dto.Problem{
			Code:    "invite_address_mismatch",
			Message: "this invitation was issued to a different address",
		})
	case errors.Is(err, organization.ErrConflict):
		writeProblem(ctx, http.StatusConflict, dto.Problem{
			Code:    "organization_conflict",
			Message: "the organization conflicts with an existing record",
		})
	case errors.Is(err, organization.ErrInvalid):
		invalidRequest(ctx, err.Error())
	case errors.Is(err, operator.ErrNoOrganization):
		writeProblem(ctx, http.StatusForbidden, dto.Problem{
			Code:    "no_organization",
			Message: "this account belongs to no organization",
		})
	default:
		return false
	}
	return true
}
