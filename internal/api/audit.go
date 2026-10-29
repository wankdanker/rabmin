package api

import (
	"context"
	"net/http"

	"github.com/wankdanker/rabmin/internal/store"
)

func (s *Server) registerAudit() {
	reg(s, route{method: http.MethodGet, path: "/v1/audit", id: "list-audit", summary: "Audit log",
		desc: "Every change made through rabmin, newest first. Page with `before` set to the last id you received.", tags: []string{"Audit"}, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			Actor  string `query:"actor"`
			Action string `query:"action" doc:"Action prefix, e.g. delete-"`
			Target string `query:"target" doc:"Substring of the target path"`
			Before int64  `query:"before"`
			Limit  int    `query:"limit" minimum:"1" maximum:"500" default:"100"`
		}) (*struct{ Body []store.AuditEntry }, error) {
			es, err := s.store.ListAudit(ctx, store.AuditQuery{Actor: in.Actor, Action: in.Action, Target: in.Target, Before: in.Before, Limit: in.Limit})
			return &struct{ Body []store.AuditEntry }{es}, apiErr(err)
		})
}
