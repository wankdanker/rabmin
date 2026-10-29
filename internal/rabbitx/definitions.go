package rabbitx

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
)

// Definitions export: GET /api/definitions or /api/definitions/{vhost}.
func (c *Client) Definitions(ctx context.Context, vhost string) (json.RawMessage, error) {
	var data []byte
	if err := c.Get(ctx, "/api/definitions"+vhostPath(vhost), &data); err != nil {
		return nil, err
	}
	return data, nil
}

// ImportDefinitions posts definitions. Import is additive and skips
// exchanges and queues that exist with different properties.
func (c *Client) ImportDefinitions(ctx context.Context, defs json.RawMessage) error {
	return c.Do(ctx, http.MethodPost, "/api/definitions", defs, nil)
}

// DefinitionChange is one object in a definitions diff.
type DefinitionChange struct {
	// ID identifies the change for a selective import.
	ID     string `json:"id"`
	Kind   string `json:"kind" doc:"vhosts, users, permissions, topic_permissions, policies, parameters, global_parameters, exchanges, queues or bindings"`
	VHost  string `json:"vhost,omitempty"`
	Name   string `json:"name"`
	Change string `json:"change" enum:"added,changed,conflict,unchanged,cluster-only" doc:"conflict: the object exists with other properties and import will not change it; cluster-only: import never deletes"`
	// Before is the object in the cluster, After the object in the file. Password hashes are masked.
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
	// Fields lists the top-level fields that differ.
	Fields []string `json:"fields"`
}

// DefinitionsDiff compares a definitions file to the cluster.
type DefinitionsDiff struct {
	Changes []DefinitionChange `json:"changes"`
	Counts  map[string]int     `json:"counts" doc:"Number of changes by change type"`
}

// definitionKinds in the order RabbitMQ imports them; the key function
// identifies an object across the two documents.
var definitionKinds = []struct {
	kind string
	key  func(o map[string]any) (vhost, name string)
}{
	{"vhosts", func(o map[string]any) (string, string) { return "", s(o["name"]) }},
	{"users", func(o map[string]any) (string, string) { return "", s(o["name"]) }},
	{"permissions", func(o map[string]any) (string, string) { return s(o["vhost"]), s(o["user"]) }},
	{"topic_permissions", func(o map[string]any) (string, string) {
		return s(o["vhost"]), s(o["user"]) + " → " + s(o["exchange"])
	}},
	{"policies", func(o map[string]any) (string, string) { return s(o["vhost"]), s(o["name"]) }},
	{"parameters", func(o map[string]any) (string, string) { return s(o["vhost"]), s(o["component"]) + "/" + s(o["name"]) }},
	{"global_parameters", func(o map[string]any) (string, string) { return "", s(o["name"]) }},
	{"exchanges", func(o map[string]any) (string, string) { return s(o["vhost"]), s(o["name"]) }},
	{"queues", func(o map[string]any) (string, string) { return s(o["vhost"]), s(o["name"]) }},
	{"bindings", func(o map[string]any) (string, string) {
		args, _ := json.Marshal(o["arguments"])
		if string(args) == "[]" || string(args) == "null" {
			args = []byte("{}")
		}
		return s(o["vhost"]), fmt.Sprintf("%s → %s %s [%s] %s", s(o["source"]), s(o["destination_type"]), s(o["destination"]), s(o["routing_key"]), args)
	}},
}

// Global parameters that identify a cluster; importing them elsewhere would be wrong.
var skipGlobal = map[string]bool{"internal_cluster_id": true, "cluster_name": true}

func s(v any) string {
	str, _ := v.(string)
	return str
}

type defDoc map[string][]map[string]any

func parseDefs(data []byte, vhost string) (defDoc, error) {
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("definitions are not a JSON object: %w", err)
	}
	doc := defDoc{}
	for _, k := range definitionKinds {
		if len(raw[k.kind]) == 0 {
			continue
		}
		var list []map[string]any
		if err := json.Unmarshal(raw[k.kind], &list); err != nil {
			return nil, fmt.Errorf("%s: %w", k.kind, err)
		}
		// A vhost export has no vhost fields; fill them in.
		if vhost != "" {
			for _, o := range list {
				if _, ok := o["vhost"]; !ok && k.kind != "vhosts" && k.kind != "users" && k.kind != "global_parameters" {
					o["vhost"] = vhost
				}
			}
		}
		doc[k.kind] = list
	}
	return doc, nil
}

func canonical(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// normalise drops fields that differ between exports without meaning a change.
func normalise(kind string, o map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range o {
		switch {
		case kind == "vhosts" && (k == "metadata" || k == "limits"):
			continue
		case kind == "users" && k == "limits" && canonical(v) == "{}":
			continue
		case k == "arguments" && (canonical(v) == "[]" || v == nil):
			v = map[string]any{}
		case kind == "users" && k == "tags":
			v = tagList(v)
		}
		out[k] = v
	}
	return out
}

func masked(kind string, o map[string]any) json.RawMessage {
	if o == nil {
		return nil
	}
	cp := map[string]any{}
	for k, v := range o {
		cp[k] = v
	}
	if kind == "users" {
		if _, ok := cp["password_hash"]; ok {
			cp["password_hash"] = "***"
		}
	}
	b, _ := json.Marshal(cp)
	return RedactURIs(b)
}

// DiffDefinitions compares a definitions file against the cluster (or one
// vhost of it when vhost is set; the file is then a vhost export).
func DiffDefinitions(cluster, file []byte, vhost string) (*DefinitionsDiff, error) {
	cur, err := parseDefs(cluster, vhost)
	if err != nil {
		return nil, fmt.Errorf("cluster definitions: %w", err)
	}
	next, err := parseDefs(file, vhost)
	if err != nil {
		return nil, err
	}
	d := &DefinitionsDiff{Changes: []DefinitionChange{}, Counts: map[string]int{}}
	for _, k := range definitionKinds {
		have := map[string]map[string]any{}
		for _, o := range cur[k.kind] {
			vh, name := k.key(o)
			have[vh+"\x00"+name] = o
		}
		seen := map[string]bool{}
		for _, o := range next[k.kind] {
			vh, name := k.key(o)
			if k.kind == "global_parameters" && skipGlobal[name] {
				continue
			}
			key := vh + "\x00" + name
			seen[key] = true
			ch := DefinitionChange{ID: k.kind + "|" + vh + "|" + name, Kind: k.kind, VHost: vh, Name: name, After: masked(k.kind, o), Fields: []string{}}
			old, ok := have[key]
			switch {
			case !ok:
				ch.Change = "added"
			default:
				ch.Before = masked(k.kind, old)
				a, b := normalise(k.kind, old), normalise(k.kind, o)
				for f := range a {
					if canonical(a[f]) != canonical(b[f]) {
						ch.Fields = append(ch.Fields, f)
					}
				}
				for f := range b {
					if _, ok := a[f]; !ok {
						ch.Fields = append(ch.Fields, f)
					}
				}
				sort.Strings(ch.Fields)
				switch {
				case len(ch.Fields) == 0:
					ch.Change = "unchanged"
				case k.kind == "exchanges" || k.kind == "queues":
					ch.Change = "conflict"
				default:
					ch.Change = "changed"
				}
			}
			d.Changes = append(d.Changes, ch)
			d.Counts[ch.Change]++
		}
		for key, o := range have {
			if seen[key] {
				continue
			}
			vh, name := k.key(o)
			if k.kind == "global_parameters" && skipGlobal[name] {
				continue
			}
			d.Changes = append(d.Changes, DefinitionChange{ID: k.kind + "|" + vh + "|" + name, Kind: k.kind, VHost: vh, Name: name,
				Change: "cluster-only", Before: masked(k.kind, o), Fields: []string{}})
			d.Counts["cluster-only"]++
		}
	}
	order := map[string]int{}
	for i, k := range definitionKinds {
		order[k.kind] = i
	}
	sort.SliceStable(d.Changes, func(i, j int) bool {
		a, b := d.Changes[i], d.Changes[j]
		if a.Kind != b.Kind {
			return order[a.Kind] < order[b.Kind]
		}
		if a.VHost != b.VHost {
			return a.VHost < b.VHost
		}
		return a.Name < b.Name
	})
	return d, nil
}

// SelectDefinitions builds a definitions document with only the objects of
// the file whose change IDs are selected (all added and changed ones when
// ids is empty). Conflicts, unchanged and cluster-only objects are never
// included. vhost fills in vhost fields for a vhost export.
func SelectDefinitions(diff *DefinitionsDiff, file []byte, vhost string, ids []string) (json.RawMessage, int, error) {
	next, err := parseDefs(file, vhost)
	if err != nil {
		return nil, 0, err
	}
	want := map[string]bool{}
	for _, id := range ids {
		want[id] = true
	}
	ok := map[string]bool{}
	for _, c := range diff.Changes {
		if (c.Change == "added" || c.Change == "changed") && (len(ids) == 0 || want[c.ID]) {
			ok[c.ID] = true
		}
	}
	out := map[string][]map[string]any{}
	n := 0
	for _, k := range definitionKinds {
		for _, o := range next[k.kind] {
			vh, name := k.key(o)
			if ok[k.kind+"|"+vh+"|"+name] {
				out[k.kind] = append(out[k.kind], o)
				n++
			}
		}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, 0, err
	}
	return json.RawMessage(strings.TrimSpace(buf.String())), n, nil
}
