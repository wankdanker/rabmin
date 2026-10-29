package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/auth"
	"github.com/wankdanker/rabmin/internal/store"
)

var tagUsers = []string{"Users & tokens"}

func (s *Server) registerUsers() {
	reg(s, route{method: http.MethodGet, path: "/v1/users", id: "list-users", summary: "List users", tags: tagUsers, role: store.RoleAdmin},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []store.User }, error) {
			us, err := s.store.ListUsers(ctx)
			return &struct{ Body []store.User }{us}, apiErr(err)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/users", id: "create-user", summary: "Create a user", tags: tagUsers, role: store.RoleAdmin, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			Body struct {
				Username string     `json:"username" minLength:"1" maxLength:"64" pattern:"^[A-Za-z0-9._@-]+$"`
				Password string     `json:"password" minLength:"8" maxLength:"256"`
				Role     store.Role `json:"role" enum:"viewer,operator,admin"`
			}
		}) (*struct{ Body store.User }, error) {
			hash, err := auth.HashPassword(in.Body.Password)
			if err != nil {
				return nil, apiErr(err)
			}
			u, err := s.store.CreateUser(ctx, in.Body.Username, hash, in.Body.Role)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body store.User }{*u}, nil
		})

	reg(s, route{method: http.MethodPatch, path: "/v1/users/{id}", id: "update-user", summary: "Update a user", tags: tagUsers, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			ID   int64 `path:"id"`
			Body struct {
				Role     *store.Role `json:"role,omitempty" enum:"viewer,operator,admin"`
				Disabled *bool       `json:"disabled,omitempty"`
				Password *string     `json:"password,omitempty" minLength:"8" maxLength:"256"`
			}
		}) (*struct{ Body store.User }, error) {
			u, err := s.store.GetUser(ctx, in.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			demoting := (in.Body.Role != nil && *in.Body.Role != store.RoleAdmin) || (in.Body.Disabled != nil && *in.Body.Disabled)
			if u.Role == store.RoleAdmin && !u.Disabled && demoting {
				n, err := s.store.CountAdmins(ctx)
				if err != nil {
					return nil, apiErr(err)
				}
				if n <= 1 {
					return nil, huma.Error409Conflict("cannot remove the last admin")
				}
			}
			if in.Body.Role != nil {
				u.Role = *in.Body.Role
			}
			if in.Body.Disabled != nil {
				u.Disabled = *in.Body.Disabled
			}
			if in.Body.Password != nil {
				if u.PasswordHash, err = auth.HashPassword(*in.Body.Password); err != nil {
					return nil, apiErr(err)
				}
			}
			if err := s.store.UpdateUser(ctx, u); err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body store.User }{*u}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/users/{id}", id: "delete-user", summary: "Delete a user", tags: tagUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID int64 `path:"id"`
		}) (*struct{}, error) {
			if principal(ctx).User.ID == in.ID {
				return nil, huma.Error409Conflict("you cannot delete yourself")
			}
			u, err := s.store.GetUser(ctx, in.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			if u.Role == store.RoleAdmin {
				n, err := s.store.CountAdmins(ctx)
				if err != nil {
					return nil, apiErr(err)
				}
				if n <= 1 && !u.Disabled {
					return nil, huma.Error409Conflict("cannot delete the last admin")
				}
			}
			return nil, apiErr(s.store.DeleteUser(ctx, in.ID))
		})

	reg(s, route{method: http.MethodGet, path: "/v1/tokens", id: "list-tokens", summary: "List API tokens",
		desc: "Lists your tokens; admins may pass all=true to list every user's tokens.", tags: tagUsers, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			All bool `query:"all"`
		}) (*struct{ Body []store.APIToken }, error) {
			p := principal(ctx)
			uid := p.User.ID
			if in.All && p.Role == store.RoleAdmin {
				uid = 0
			}
			toks, err := s.store.ListTokens(ctx, uid)
			return &struct{ Body []store.APIToken }{toks}, apiErr(err)
		})

	type createdToken struct {
		store.APIToken
		Token string `json:"token" doc:"The token value. It is only shown once."`
	}
	reg(s, route{method: http.MethodPost, path: "/v1/tokens", id: "create-token", summary: "Create an API token", tags: tagUsers, role: store.RoleViewer, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			Body struct {
				Name      string     `json:"name" minLength:"1" maxLength:"100"`
				Role      store.Role `json:"role,omitempty" enum:"viewer,operator,admin" doc:"Limit the token to this role; defaults to your role"`
				ExpiresIn int        `json:"expiresInDays,omitempty" minimum:"0" maximum:"3650" doc:"0 means never"`
			}
		}) (*struct{ Body createdToken }, error) {
			p := principal(ctx)
			if p.Method != "session" {
				return nil, huma.Error403Forbidden("API tokens can only be created from a login session")
			}
			role := in.Body.Role
			if role == "" {
				role = p.User.Role
			}
			role = store.Min(role, p.User.Role)
			var exp *time.Time
			if in.Body.ExpiresIn > 0 {
				t := time.Now().AddDate(0, 0, in.Body.ExpiresIn)
				exp = &t
			}
			tok, display := auth.NewAPIToken()
			t, err := s.store.CreateToken(ctx, p.User.ID, in.Body.Name, display, auth.HashToken(tok), role, exp)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body createdToken }{createdToken{APIToken: *t, Token: tok}}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/tokens/{id}", id: "delete-token", summary: "Revoke an API token", tags: tagUsers, role: store.RoleViewer, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ID int64 `path:"id"`
		}) (*struct{}, error) {
			p := principal(ctx)
			uid := p.User.ID
			if p.Role == store.RoleAdmin {
				uid = 0
			}
			return nil, apiErr(s.store.DeleteToken(ctx, in.ID, uid))
		})
}
