package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/kirchDev/terraform-provider-linear/internal/client"
)

// Linear's mutations are strikingly regular: for an entity `x` there is
// xCreate(input: XCreateInput!), xUpdate(id: String!, input: XUpdateInput!) and
// xDelete(id: String!) (or xArchive where Linear soft-deletes), each returning
// an XPayload whose `x` field carries the entity. Reads go through the top-level
// x(id: String!) query. entity captures that regularity so a resource file only
// has to declare its selection set and its input mapping.

// entity is the GraphQL surface of one Linear entity.
type entity struct {
	// name is the camelCase entity name — the query field, the mutation prefix
	// and the field the payload wraps the entity in, e.g. "issueLabel".
	name string
	// typePrefix is the PascalCase input-type prefix, e.g. "IssueLabel" for
	// IssueLabelCreateInput. Derived from name when empty.
	typePrefix string
	// fields is the selection set every read uses.
	fields string
	// deleteVerb is the mutation suffix that removes the entity — "Delete" by
	// default, "Archive" for the entities Linear only soft-deletes.
	deleteVerb string
	// extraCreateArgs are additional create-mutation arguments beyond `input`,
	// keyed by argument name and holding the GraphQL type, e.g.
	// {"replaceTeamLabels": "Boolean"}.
	extraCreateArgs map[string]string
}

func (e entity) prefix() string {
	if e.typePrefix != "" {
		return e.typePrefix
	}
	return strings.ToUpper(e.name[:1]) + e.name[1:]
}

func (e entity) removeVerb() string {
	if e.deleteVerb != "" {
		return e.deleteVerb
	}
	return "Delete"
}

// create runs xCreate and decodes the created entity into out. extraArgs holds
// values for the arguments declared in extraCreateArgs.
func (e entity) create(ctx context.Context, c *client.Client, in map[string]any, extraArgs map[string]any, out any) error {
	op := e.name + "Create"

	params := []string{"$input: " + e.prefix() + "CreateInput!"}
	args := []string{"input: $input"}
	vars := map[string]any{"input": in}
	for arg, typ := range e.extraCreateArgs {
		v, ok := extraArgs[arg]
		if !ok {
			continue
		}
		params = append(params, "$"+arg+": "+typ)
		args = append(args, arg+": $"+arg)
		vars[arg] = v
	}

	doc := fmt.Sprintf("mutation %s(%s) {\n  %s(%s) {\n    %s { %s }\n  }\n}",
		op, strings.Join(params, ", "), op, strings.Join(args, ", "), e.name, e.fields)
	return e.mutateInto(ctx, c, doc, vars, op, out)
}

// update runs xUpdate and decodes the updated entity into out. Callers pass nil
// and read the entity back instead, because an xUpdate payload does not always
// reflect the write it is answering. The document still selects the full field
// set even so: that is what keeps the mutation's selection set under the same
// validation the read's is, and Linear returns the payload either way.
func (e entity) update(ctx context.Context, c *client.Client, id string, in map[string]any, out any) error {
	op := e.name + "Update"
	doc := fmt.Sprintf("mutation %s($id: String!, $input: %sUpdateInput!) {\n  %s(id: $id, input: $input) {\n    %s { %s }\n  }\n}",
		op, e.prefix(), op, e.name, e.fields)
	return e.mutateInto(ctx, c, doc, map[string]any{"id": id, "input": in}, op, out)
}

// read runs the x(id:) query and decodes the entity into out. A missing entity
// surfaces as an error NotFound reports true for.
func (e entity) read(ctx context.Context, c *client.Client, id string, out any) error {
	doc := fmt.Sprintf("query %s($id: String!) {\n  %s(id: $id) { %s }\n}", e.name, e.name, e.fields)

	var data map[string]json.RawMessage
	if err := c.Query(ctx, doc, map[string]any{"id": id}, &data); err != nil {
		return err
	}
	return decodeField(data, e.name, out)
}

// remove runs xDelete (or xArchive). A NotFound is swallowed — a Delete that
// finds the entity already gone has done its job.
func (e entity) remove(ctx context.Context, c *client.Client, id string) error {
	op := e.name + e.removeVerb()
	doc := fmt.Sprintf("mutation %s($id: String!) {\n  %s(id: $id) { success }\n}", op, op)
	if err := c.Mutate(ctx, doc, map[string]any{"id": id}, nil); err != nil && !client.NotFound(err) {
		return err
	}
	return nil
}

func (e entity) mutateInto(ctx context.Context, c *client.Client, doc string, vars map[string]any, op string, out any) error {
	var data map[string]json.RawMessage
	if err := c.Mutate(ctx, doc, vars, &data); err != nil {
		return err
	}
	if out == nil {
		return nil
	}
	var payload map[string]json.RawMessage
	if err := decodeField(data, op, &payload); err != nil {
		return err
	}
	return decodeField(payload, e.name, out)
}

// decodeField pulls one field out of a decoded GraphQL object. A field that is
// absent or JSON null means the entity does not exist, which Read has to see as
// a not-found so it can drop the resource from state.
func decodeField(obj map[string]json.RawMessage, field string, out any) error {
	raw, ok := obj[field]
	if !ok || string(raw) == "null" {
		return notFoundError(field)
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return fmt.Errorf("decoding %s: %w", field, err)
	}
	return nil
}

// notFoundError is what a read answers with when the entity is not there. It is
// shaped like Linear's own EntityNotFoundError, because that is what NotFound
// looks for and what makes Read drop the resource from state rather than fail
// the plan. A read that has to search for its entity — one Linear exposes no
// root query for — raises it itself, having searched and not found it.
func notFoundError(field string) error {
	return &client.APIError{
		Operation: field,
		Errors: []client.GraphQLError{{
			Message:   fmt.Sprintf("%s not found", field),
			Extension: map[string]any{"type": "EntityNotFoundError"},
		}},
	}
}

// connection runs a paginated collection query and decodes every node into out
// (a pointer to a slice). params declares the query variables (e.g.
// "$filter: WorkflowStateFilter"), args the arguments passed to the collection
// field (e.g. "filter: $filter"); both may be empty.
//
// Pagination is followed to the end rather than capped: a workspace with more
// than one page of labels must not silently read half of them. Linear's page
// maximum is 250.
func connection[T any](ctx context.Context, c *client.Client, field, params, args, fields string, vars map[string]any, out *[]T) error {
	paramDecl := "$after: String"
	if params != "" {
		paramDecl = params + ", " + paramDecl
	}
	argList := "first: 250, after: $after"
	if args != "" {
		argList = args + ", " + argList
	}
	doc := fmt.Sprintf("query %s(%s) {\n  %s(%s) {\n    nodes { %s }\n    pageInfo { hasNextPage endCursor }\n  }\n}",
		field, paramDecl, field, argList, fields)

	var after *string
	for {
		pageVars := map[string]any{"after": after}
		for k, v := range vars {
			pageVars[k] = v
		}

		var data map[string]json.RawMessage
		if err := c.Query(ctx, doc, pageVars, &data); err != nil {
			return err
		}
		var conn struct {
			Nodes    []T `json:"nodes"`
			PageInfo struct {
				HasNextPage bool    `json:"hasNextPage"`
				EndCursor   *string `json:"endCursor"`
			} `json:"pageInfo"`
		}
		if err := decodeField(data, field, &conn); err != nil {
			return err
		}
		*out = append(*out, conn.Nodes...)

		if !conn.PageInfo.HasNextPage || conn.PageInfo.EndCursor == nil {
			return nil
		}
		after = conn.PageInfo.EndCursor
	}
}
